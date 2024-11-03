package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"node-analysis/zmq"
	"os"
	"time"

	"github.com/btcsuite/btcd/btcjson"
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

const (
	host     = "localhost"
	user     = "bitcoin"
	password = "bitcoin"
	rpcPort  = 18443
	zmqPort  = 29000
)

func run() error {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	// bitcoind, err := bitcoin.New(host, port, user, password, false)
	// if err != nil {
	// 	log.Fatalln("Failed to create bitcoind instance:", err)
	// }

	// inf, err := bitcoind.GetNetworkInfo()
	// if err != nil {
	// 	log.Fatalln(err)
	// }

	// logger.Info("inf", "balance", inf.ProtocolVersion)

	zmqURLString := fmt.Sprintf("zmq://%s:%d", host, zmqPort)

	zmqURL, err := url.Parse(zmqURLString)
	if err != nil {
		return fmt.Errorf("failed to parse zmq URL: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zmqSubscriber, err := zmq.NewZMQWithContext(ctx, host, zmqPort, logger)
	if err != nil {
		return err
	}

	zmqClient := zmq.NewZMQClient(zmqURL, logger)

	zmqClient.Start(zmqSubscriber)

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

	logger.Info("mining info", "blocks", info.Blocks, "current block size", info.CurrentBlockSize)

	walletName := "test-2"

	wallet, err := client.CreateWallet(walletName)
	if err != nil {
		rpcErr, ok := err.(*btcjson.RPCError)
		if ok && rpcErr.Code == btcjson.ErrRPCWallet {
			logger.Warn("failed to create wallet - already exists", "err", err)
		} else {
			return err
		}
	} else {
		logger.Info("wallet created", "name", wallet.Name)
	}

	address, err := client.GetNewAddress(walletName)
	if err != nil {
		return fmt.Errorf("failed to get new address: %v", err)
	}

	logger.Info("wallet address", "address", address.EncodeAddress())

	hashes, err := client.GenerateToAddress(101, address, ptrTo(int64(3)))
	if err != nil {
		return fmt.Errorf("failed to gnereate to address: %v", err)
	}
	for _, hash := range hashes {
		logger.Info("hash", "hex string", hash.String())
	}

	// _, err = client.Generate(5)
	// if err != nil {
	// 	return fmt.Errorf("failed to generate 1: %v", err)
	// }

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

	time.Sleep(5 * time.Second)

	return nil
}

func ptrTo[T any](v T) *T {
	return &v
}
