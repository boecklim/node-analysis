package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"node-analysis/broadcaster"
	"node-analysis/node_client/bsv"
	"node-analysis/node_client/btc"
	"node-analysis/zmq"
	"os"
	"time"

	slogmulti "github.com/samber/slog-multi"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/ordishs/go-bitcoin"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("failed to run listener: %v", err)
	}

	os.Exit(0)
}

const (
	pubhashblock   = "hashblock"
	pubhashtx      = "hashtx"
	bsvBlockchain  = "bsv"
	btcBlockchain  = "btc"
	hostDefault    = "localhost"
	zmqPortDefault = 29000
	rpcUser        = "bitcoin"
	rpcPassword    = "bitcoin"
	rpcHostDefault = "localhost"
	rpcPortDefault = 18443
)

func run() error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	zmqPort := flag.Int("port", zmqPortDefault, "port of listener client")
	if zmqPort == nil {
		return errors.New("rpc port not given")
	}

	zmqHost := flag.String("host", hostDefault, "host of listener client")
	if zmqHost == nil {
		return errors.New("rpc host not given")
	}

	rpcPort := flag.Int("rpc-port", rpcPortDefault, "port of RPC client")
	if rpcPort == nil {
		return errors.New("rpc port not given")
	}

	rpcHost := flag.String("rpc-host", rpcHostDefault, "host of RPC client")
	if rpcHost == nil {
		return errors.New("rpc host not given")
	}

	blockchain := flag.String("blockchain", "btc", "one of btc | bsv")
	if blockchain == nil {
		return errors.New("blockchain not given")
	}

	flag.Parse()

	zmqURLString := fmt.Sprintf("zmq://%s:%d", *zmqHost, *zmqPort)

	zmqURL, err := url.Parse(zmqURLString)
	if err != nil {
		return fmt.Errorf("failed to parse zmq URL: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zmqSubscriber, err := zmq.NewZMQWithContext(ctx, *zmqHost, *zmqPort, logger)
	if err != nil {
		return err
	}

	zmqClient := zmq.NewZMQClient(zmqURL, logger)
	var client broadcaster.RPCClient
	blockChan := make(chan zmq.BlockEvent, 5000)

	switch *blockchain {
	case btcBlockchain:
		btcClient, err := rpcclient.New(&rpcclient.ConnConfig{
			Host:         fmt.Sprintf("%s:%d", *rpcHost, *rpcPort),
			User:         rpcUser,
			Pass:         rpcPassword,
			HTTPPostMode: true,
			DisableTLS:   true,
		}, nil)
		if err != nil {
			return fmt.Errorf("failed to create btc rpc client: %v", err)
		}
		info, err := btcClient.GetMiningInfo()
		if err != nil {
			return fmt.Errorf("failed to get info: %v", err)
		}

		networkInfo, err := btcClient.GetNetworkInfo()
		if err != nil {
			return err
		}

		logger.Info("mining info", "blocks", info.Blocks, "current block size", info.CurrentBlockSize)
		logger.Info("network info", "version", networkInfo.Version)
		client, err = btc.New(btcClient, logger)
		if err != nil {
			return fmt.Errorf("failed to create rpc client: %v", err)
		}
	case bsvBlockchain:
		rpcURL, err := url.Parse(fmt.Sprintf("rpc://%s:%s@%s:%d", rpcUser, rpcPassword, *rpcHost, *rpcPort))
		if err != nil {
			return fmt.Errorf("failed to parse node rpc url: %w", err)
		}

		bsvClient, err := bitcoin.NewFromURL(rpcURL, false)
		if err != nil {
			return fmt.Errorf("failed to create bitcoin client: %w", err)
		}

		info, err := bsvClient.GetMiningInfo()
		if err != nil {
			return fmt.Errorf("failed to get info: %v", err)
		}

		networkInfo, err := bsvClient.GetNetworkInfo()
		if err != nil {
			return err
		}

		logger.Info("mining info", "blocks", info.Blocks, "current block size", info.CurrentBlockSize)
		logger.Info("network info", "version", networkInfo.Version)

		client, err = bsv.New(bsvClient, logger)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("given blockchain %s not valid - has to be either %s or %s", *blockchain, bsvBlockchain, btcBlockchain)
	}

	logFile, err := os.OpenFile("output.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer logFile.Close()

	go func() {
		listenerLogger := slog.New(
			slogmulti.Fanout(
				slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}),
				slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
			),
		)
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case blockEvent := <-blockChan:
				var blockHash *chainhash.Hash
				var err error
				listenerLogger.Debug("block received", "hash", blockEvent.Hash)
				blockHash, err = chainhash.NewHashFromStr(blockEvent.Hash)
				if err != nil {
					listenerLogger.Error("failed to create hash from hex string", "err", err)
					continue
				}

				sizeBytes, nrTxs, err := client.GetBlockSize(blockHash)
				if err != nil {
					listenerLogger.Error("failed to get block for block hash", "hash", blockHash.String(), "err", err)
					continue
				}

				listenerLogger.Info("block", "hash", blockEvent.Hash, "timestamp", blockEvent.Timestamp.Format(time.RFC3339), "txs", nrTxs, "size", sizeBytes)
			}
		}
	}()

	ch := make(chan []string, 1000)

	if err := zmqSubscriber.Subscribe(pubhashblock, ch); err != nil {
		return err
	}

	err = zmqSubscriber.Start(ctx)
	if err != nil {
		return err
	}
	err = zmqClient.Start(ctx, zmqSubscriber, blockChan, ch)
	if err != nil {
		return err
	}

	return nil
}
