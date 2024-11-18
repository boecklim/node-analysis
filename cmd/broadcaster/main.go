package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"node-analysis/broadcaster"
	"os"
	"os/signal"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/rpcclient"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("failed to run: %v", err)
	}

	os.Exit(0)
}

const (
	host     = "localhost"
	user     = "bitcoin"
	password = "bitcoin"
	rpcPort  = 18443
	zmqPort  = 29000
)

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%d", host, rpcPort),
		User:         user,
		Pass:         password,
		HTTPPostMode: true,
		DisableTLS:   true,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create rpc client: %v", err)
	}
	info, err := client.GetMiningInfo()
	if err != nil {
		return fmt.Errorf("failed to get info: %v", err)
	}

	networkInfo, err := client.GetNetworkInfo()
	if err != nil {
		return err
	}

	logger.Info("mining info", "blocks", info.Blocks, "current block size", info.CurrentBlockSize)
	logger.Info("network info", "version", networkInfo.Version)

	privKeyBytes, err := hex.DecodeString("13d2c242e1286ce48b86d51742e4a9a44398e36a0400fdb87425a014538a7413")
	if err != nil {
		return err
	}
	privKey, pubKey := btcec.PrivKeyFromBytes(privKeyBytes)
	address, err := btcutil.NewAddressPubKey(pubKey.SerializeCompressed(),
		&chaincfg.RegressionNetParams)
	if err != nil {
		return err
	}
	logger.Info("address", "address", address.EncodeAddress())

	p, err := broadcaster.New(client, logger, address, privKey)
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
		err = p.Start(20, 100)
		logger.Info("Starting broadcaster")
		doneChan <- err // Send the completion or error signal
	}()

	select {
	case <-signalChan:
		// If an interrupt signal is received
		logger.Info("Shutdown signal received. Shutting down the rate broadcaster.")
	case err := <-doneChan:
		if err != nil {
			logger.Error("Error during broadcasting", slog.String("err", err.Error()))
		}
	}

	// Shutdown the broadcaster in all cases
	p.Shutdown()
	logger.Info("Broadcasting shutdown complete")
	return nil
}
