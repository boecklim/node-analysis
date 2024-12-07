package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"node-analysis/node_client/bsv"
	"node-analysis/node_client/btc"
	"node-analysis/processor"
	"node-analysis/zmq"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/lmittmann/tint"
	"github.com/ordishs/go-bitcoin"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("failed to run: %v", err)
	}

	os.Exit(0)
}

const (
	rpcUser        = "bitcoin"
	rpcPassword    = "bitcoin"
	rpcHostDefault = "localhost"
	rpcPortDefault = 18443
	bsvBlockchain  = "bsv"
	btcBlockchain  = "btc"

	pubhashblockTopic = "hashblock"
	pubhashtxTopic    = "hashtx"
	hostDefault       = "localhost"
	zmqPortDefault    = 29000
)

func run() error {
	blockchain := flag.String("blockchain", "btc", "one of btc | bsv")
	if blockchain == nil {
		return errors.New("blockchain not given")
	}

	zmqPort := flag.Int("zmq-port", zmqPortDefault, "zmq port")
	if zmqPort == nil {
		return errors.New("zmq port not given")
	}

	rpcPort := flag.Int("rpc-port", rpcPortDefault, "RPC port")
	if rpcPort == nil {
		return errors.New("rpc port not given")
	}

	host := flag.String("host", rpcHostDefault, "host")
	if host == nil {
		return errors.New("rpc host not given")
	}

	outputPath := flag.String("output", "output.log", "path to output file of listener e.g. ./results/output.log")
	if outputPath == nil {
		return errors.New("output not given")
	}

	txsRate := flag.Int64("rate", 5, "rate in txs per second")
	if txsRate == nil {
		return errors.New("rate not given")
	}

	limit := flag.Int64("limit", 20, "limit of txs at which to stop broadcasting")
	if limit == nil {
		return errors.New("limit not given")
	}

	generateBlocks := flag.Duration("gen-blocks", 0, "time interval in which to generate a new block - for value 0 no blocks are going to be generated. Valid time units are s, m, h")
	if generateBlocks == nil {
		return errors.New("generate block interval not given")
	}

	startAt := flag.String("start-at", "2024-10-01T00:00:00+01:00", "time at which to start - format RFC3339: e.g. 2024-12-02T21:16:00+01:00")
	if startAt == nil {
		return errors.New("startAt not given")
	}

	flag.Parse()

	if *generateBlocks == 0 {
		generateBlocks = nil
	}

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelInfo, TimeFormat: time.Kitchen}))

	waitUntil, err := time.Parse(time.RFC3339, *startAt)
	if err != nil {
		return err
	}

	startTimer := time.NewTimer(time.Until(waitUntil))

	var client processor.RPCClient

	switch *blockchain {
	case btcBlockchain:
		btcClient, err := rpcclient.New(&rpcclient.ConnConfig{
			Host:         fmt.Sprintf("%s:%d", *host, *rpcPort),
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
		rpcURL, err := url.Parse(fmt.Sprintf("rpc://%s:%s@%s:%d", rpcUser, rpcPassword, *host, *rpcPort))
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

	path := filepath.Dir(*outputPath)
	err = os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create path: %v", err)
	}

	logFile, err := os.OpenFile(*outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer logFile.Close()

	messageChan := make(chan []string, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zmqSubscriber, err := zmq.New(ctx, *host, *zmqPort, logger)
	if err != nil {
		return err
	}

	err = zmqSubscriber.Subscribe(pubhashblockTopic, messageChan)
	if err != nil {
		return err
	}

	err = zmqSubscriber.Start(ctx)
	if err != nil {
		return err
	}

	broadcaster, err := processor.NewBroadcaster(ctx, client, logger)
	if err != nil {
		return err
	}

	logger.Info("Preparing utxos")
	var ignoredBlockHashes map[string]struct{}
	ignoredBlockHashes, err = broadcaster.PrepareUtxos(10000)
	if err != nil {
		return err
	}
	newBlockCh := make(chan struct{}, 100)

	miner := processor.NewMiner(client, logger)

	listener := processor.NewListener(client)

	logger.Info("Starting listening")
	listener.Start(ctx, ignoredBlockHashes, messageChan, newBlockCh, logFile)

	logger.Info("Waiting to start", "until", waitUntil.String())
	<-startTimer.C

	logger.Info("Starting mining")
	miner.Start(ctx, *generateBlocks, newBlockCh)

	doneChan := make(chan error)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt) // Listen for Ctrl+C

	go func() {
		logger.Info("Starting broadcasting")
		err = broadcaster.Start(*txsRate, *limit)
		doneChan <- err
	}()

	select {
	case <-signalChan:
		logger.Info("Shutdown signal received. Shutting down the rate broadcaster.")
		break
	case err = <-doneChan:
		if err != nil {
			logger.Error("Error during broadcasting", slog.String("err", err.Error()))
		}
	}

	broadcaster.Shutdown()
	logger.Info("Broadcasting shutdown complete")
	return nil
}
