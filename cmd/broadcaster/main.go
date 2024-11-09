package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"node-analysis/utils"
	"os"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
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

	fmt.Println(networkInfo.Version)

	logger.Info("mining info", "blocks", info.Blocks, "current block size", info.CurrentBlockSize)

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

	blockHashes, err := client.GenerateToAddress(101, address, nil)
	if err != nil {
		return fmt.Errorf("failed to gnereate to address: %v", err)
	}
	for _, hash := range blockHashes {
		logger.Info("block hash", "hex string", hash.String())
	}

	lastBlock, err := client.GetBlock(blockHashes[0])
	if err != nil {
		return err
	}

	txHash := lastBlock.Transactions[0].TxHash()

	// USE GETTXOUT https://bitcoin.stackexchange.com/questions/117919/bitcoin-cli-listunspent-returns-empty-list
	txOut, err := client.GetTxOut(&txHash, 0, false)
	if err != nil {
		return err
	}

	tx, err := utils.PayToAddress(address, &txHash, txOut, privKey)
	if err != nil {
		return err
	}

	sentTxHash, err := client.SendRawTransaction(tx, false)
	if err != nil {
		return err
	}

	logger.Info("sent raw tx", "hash", sentTxHash.String())

	return nil
}
