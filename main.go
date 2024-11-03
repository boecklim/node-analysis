package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/lmittmann/tint"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("failed to run: %v", err)
	}

	os.Exit(0)
}

func run() error {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	client, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:         "localhost:18443",
		User:         "bitcoin",
		Pass:         "bitcoin",
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

	fmt.Println(info.Blocks)
	logger.Info("mining info", "blocks", info.Blocks, "current block size", info.CurrentBlockSize)

	wallet, err := client.CreateWallet("test-1")
	if err != nil {
		logger.Error("failed to create wallet", "err", err)
	} else {
		logger.Info("wallet created", "name", wallet.Name)
	}

	address, err := client.GetNewAddress("test-1")
	if err != nil {
		return fmt.Errorf("failed to get new address: %v", err)
	}

	logger.Info("wallet address", "address", address.EncodeAddress())

	// _, err = client.GenerateToAddress(101, address, ptrTo(int64(3)))
	// if err != nil {
	// 	return fmt.Errorf("failed to gnereate to address: %v", err)
	// }

	// isMining, err := client.GetGenerate()
	// if err != nil {
	// 	return fmt.Errorf("failed to get generate: %v", err)
	// }

	// logger.Info("generate", "is mining", isMining)

	_, err = client.Generate(5)
	if err != nil {
		return fmt.Errorf("failed to generate 1: %v", err)
	}

	results, err := client.ListReceivedByAddress()
	if err != nil {
		return fmt.Errorf("failed to list received by address: %v", err)
	}

	for _, result := range results {
		logger.Info("result", "account", result.Account, "address", result.Address, "amount", result.Amount)
	}

	unspent, err := client.ListUnspent()
	if err != nil {
		return fmt.Errorf("failed to list received by address: %v", err)
	}
	for _, u := range unspent {
		logger.Info("unspent", "account", u.Account, "address", u.Address, "amount", u.Amount)
	}

	return nil
}

func ptrTo[T any](v T) *T {
	return &v
}
