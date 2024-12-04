package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"node-analysis/zmq"
	"os"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("failed to run listener: %v", err)
	}

	os.Exit(0)
}

const (
	hostDefault    = "localhost"
	zmqPortDefault = 29000
	rpcUser        = "bitcoin"
	rpcPassword    = "bitcoin"
	rpcHostDefault = "localhost"
	rpcPortDefault = 18443
)

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

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

	blockChan := make(chan zmq.BlockEvent, 5000)

	// btcClient, err := rpcclient.New(&rpcclient.ConnConfig{
	// 	Host:         fmt.Sprintf("%s:%d", *rpcHost, *rpcPort),
	// 	User:         rpcUser,
	// 	Pass:         rpcPassword,
	// 	HTTPPostMode: true,
	// 	DisableTLS:   true,
	// }, nil)
	// if err != nil {
	// 	return fmt.Errorf("failed to create btc rpc client: %v", err)
	// }
	// info, err := btcClient.GetMiningInfo()
	// if err != nil {
	// 	return fmt.Errorf("failed to get info: %v", err)
	// }

	// networkInfo, err := btcClient.GetNetworkInfo()
	// if err != nil {
	// 	return err
	// }

	// logger.Info("mining info", "blocks", info.Blocks, "current block size", info.CurrentBlockSize)
	// logger.Info("network info", "version", networkInfo.Version)

	go func() {
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case blockEvent := <-blockChan:
				// var blockHash *chainhash.Hash
				// var err error
				logger.Info("block received", "hash", blockEvent.Hash)
				// blockHash, err = chainhash.NewHashFromStr(blockEvent.Hash)
				// if err != nil {
				// 	logger.Error("failed to create hash from hex string", "err", err)
				// 	continue
				// }

				// block, err := btcClient.GetBlock(blockHash)
				// if err != nil {
				// 	logger.Error("failed to get block for block hash", "hash", blockHash.String(), "err", err)
				// 	continue
				// }

				// logger.Info("block", "hash", blockEvent.Hash, "timestamp", blockEvent.Timestamp.String(), "txs", len(block.Transactions))
			}
		}
	}()

	err = zmqClient.Start(ctx, zmqSubscriber, blockChan)
	if err != nil {
		return err
	}

	return nil
}
