package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"node-analysis/broadcaster"
	"node-analysis/node_client/bsv"
	"node-analysis/node_client/btc"
	"os"
	"os/signal"

	"github.com/btcsuite/btcd/rpcclient"
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
)

func run() error {
	blockchain := flag.String("blockchain", "btc", "one of btc | bsv")
	if blockchain == nil {
		return errors.New("blockchain not given")
	}

	rpcPort := flag.Int("port", rpcPortDefault, "port of RPC client")
	if rpcPort == nil {
		return errors.New("rpc port not given")
	}

	rpcHost := flag.String("host", rpcHostDefault, "host of RPC client")
	if rpcHost == nil {
		return errors.New("rpc host not given")
	}

	txsRate := flag.Int64("rate", 5, "rate in txs per second")
	if txsRate == nil {
		return errors.New("rate not given")
	}

	limit := flag.Int64("limit", 20, "limit of txs at which to stop broadcastiong")
	if limit == nil {
		return errors.New("limit not given")
	}

	generateBlocks := flag.Int64("gen-blocks", 0, "interval of seconds in which to generate a new block - for value 0 no blocks are going to be generated")
	if generateBlocks == nil {
		return errors.New("generate block interval not given")
	}

	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var client broadcaster.RPCClient

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
		client, err = btc.New(btcClient)
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

		client, err = bsv.New(bsvClient)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("given blockchain %s not valid - has to be either %s or %s", *blockchain, bsvBlockchain, btcBlockchain)
	}

	p, err := broadcaster.New(client, logger)
	if err != nil {
		return err
	}

	err = p.PrepareUtxos()
	if err != nil {
		return err
	}

	doneChan := make(chan error) // Channel to signal the completion of Start
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt) // Listen for Ctrl+C

	go func() {
		// Start the broadcasting process
		err = p.Start(*txsRate, *limit, *generateBlocks)
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
