package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/lmittmann/tint"
	slogmulti "github.com/samber/slog-multi"

	"github.com/boecklim/node-analysis/pkg/broadcaster"
	"github.com/boecklim/node-analysis/pkg/listener"
	"github.com/boecklim/node-analysis/pkg/miner"
	"github.com/boecklim/node-analysis/pkg/node_client"
	"github.com/boecklim/node-analysis/pkg/zmq"
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
	zmqPortDefault    = 29000
)

func run() error {
	var err error

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

	outputPath := flag.String("output", "", "path to output file of listener e.g. ./results/output.log")
	if outputPath == nil {
		return errors.New("output not given")
	}

	txsRate := flag.Int64("rate", 5, "rate in txs per second")
	if txsRate == nil {
		return errors.New("rate not given")
	}

	limit := flag.Duration("limit", 10*time.Minute, "time limit after which to stop broadcasting")
	if limit == nil {
		return errors.New("limit not given")
	}

	wait := flag.Duration("wait", 0*time.Second, "time to wait before utxo preparation")
	if wait == nil {
		return errors.New("wait not given")
	}

	generateBlocks := flag.Duration("gen-blocks", 0, "time interval in which to generate a new block - for value 0 no blocks are going to be generated. Valid time units are s, m, h")
	if generateBlocks == nil {
		return errors.New("generate block interval not given")
	}

	startAt := flag.String("start-at", "", "time at which to start - format RFC3339: e.g. 2024-12-02T21:16:00+01:00")
	if startAt == nil {
		return errors.New("startAt not given")
	}

	flag.Parse()

	if *generateBlocks == 0 {
		generateBlocks = nil
	}

	var startBroadcastingAt time.Time
	if *startAt == "" {
		startBroadcastingAt = time.Now().Round(5 * time.Second).Add(90 * time.Second)
	} else {
		startBroadcastingAt, err = time.Parse(time.RFC3339, *startAt)
		if err != nil {
			return err
		}
	}

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelInfo, TimeFormat: time.RFC3339}))

	btcClient, err := node_client.New(*host, *rpcPort, rpcUser, rpcPassword, slog.Default())
	if err != nil {
		return err
	}
	var proc *node_client.Processor

	switch *blockchain {
	case btcBlockchain:
		proc, err = node_client.NewProcessor(btcClient, logger, false)
	case bsvBlockchain:
		proc, err = node_client.NewProcessor(btcClient, logger, true)
	default:
		return fmt.Errorf("given blockchain %s not valid - has to be either %s or %s", *blockchain, bsvBlockchain, btcBlockchain)
	}
	if err != nil {
		return err
	}

	info, err := btcClient.GetMiningInfo()
	if err != nil {
		return fmt.Errorf("failed to get info: %v", err)
	}
	logger.Info("mining info", "blocks", info.Blocks, "errors", info.Errors)

	networkInfo, err := btcClient.GetNetworkInfo()
	if err != nil {
		return err
	}

	logger.Info("network info", "version", networkInfo.Version)

	if err != nil {
		return fmt.Errorf("failed to create rpc client: %v", err)
	}
	var broadcasterLogger *slog.Logger
	if *outputPath != "" {
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
		broadcasterLogger = slog.New(
			slogmulti.Fanout(
				slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}),
				tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelInfo, TimeFormat: time.Kitchen}),
			),
		)
	} else {
		broadcasterLogger = logger
	}

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

	newBroadcaster, err := broadcaster.NewBroadcaster(proc)
	if err != nil {
		return err
	}

	logger.Info("Waiting to prepare utxos", "until", startBroadcastingAt.Add(-1**wait).String())

	time.Sleep(time.Until(startBroadcastingAt.Add(-1 * *wait)))

	logger.Info("Preparing utxos")
	err = newBroadcaster.PrepareUtxos(10000)
	if err != nil {
		return err
	}
	newBlockCh := make(chan string, 100)

	newMiner := miner.New(proc)

	newListener := listener.New(proc)

	logger.Info("Starting listening")

	newListener.Start(ctx, messageChan, newBlockCh, broadcasterLogger, startBroadcastingAt)

	logger.Info("Starting mining")
	newMiner.Start(ctx, *generateBlocks, newBlockCh, broadcasterLogger, startBroadcastingAt)

	doneChan := make(chan error)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt) // Listen for Ctrl+C

	go func() {
		err = newBroadcaster.Start(*txsRate, *limit, broadcasterLogger)
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

	newBroadcaster.Shutdown()
	logger.Info("Broadcasting shutdown complete")
	return nil
}
