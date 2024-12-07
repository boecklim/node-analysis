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
	"node-analysis/listener"
	"node-analysis/node_client/bsv"
	"node-analysis/node_client/btc"
	"node-analysis/zmq"
	"os"
	"os/signal"
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

	outputFile := flag.String("output", "output.log", "filename where to store output")
	if outputFile == nil {
		return errors.New("outputFile not given")
	}

	txsRate := flag.Int64("rate", 5, "rate in txs per second")
	if txsRate == nil {
		return errors.New("rate not given")
	}

	limit := flag.Int64("limit", 20, "limit of txs at which to stop broadcastiong")
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

	var client broadcaster.RPCClient

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

	logFile, err := os.OpenFile(*outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer logFile.Close()

	blockChannel := make(chan []string, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zmqSubscriber, err := zmq.New(ctx, *host, *zmqPort, logger)
	if err != nil {
		return err
	}
	if err := zmqSubscriber.Subscribe(pubhashblockTopic, blockChannel); err != nil {
		return err
	}

	err = zmqSubscriber.Start(ctx)
	if err != nil {
		return err
	}

	p, err := broadcaster.New(client, logger)
	if err != nil {
		return err
	}

	logger.Info("Preparing utxos")
	err = p.PrepareUtxos(2000)
	if err != nil {
		return err
	}

	lis := listener.New(client)

	logger.Info("Starting listening")
	lis.Start(ctx, blockChannel, logFile)

	logger.Info("Waiting to start broadcasting", "until", waitUntil.String())
	<-startTimer.C

	doneChan := make(chan error) // Channel to signal the completion of Start
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt) // Listen for Ctrl+C

	go func() {
		// Start the broadcasting process
		err = p.Start(*txsRate, *limit, generateBlocks)
		logger.Info("Starting broadcaster")
		doneChan <- err // Send the completion or error signal
	}()

	select {
	case <-signalChan:
		// If an interrupt signal is received
		logger.Info("Shutdown signal received. Shutting down the rate broadcaster.")
	case err = <-doneChan:
		if err != nil {
			logger.Error("Error during broadcasting", slog.String("err", err.Error()))
		}
	}

	// Shutdown the broadcaster in all cases
	p.Shutdown()
	logger.Info("Broadcasting shutdown complete")
	return nil
}
