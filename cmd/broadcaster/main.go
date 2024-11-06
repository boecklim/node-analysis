package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"node-analysis/utils"
	"os"

	ec "github.com/bitcoin-sv/go-sdk/primitives/ec"
	sdkTx "github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/bitcoin-sv/go-sdk/transaction/template/p2pkh"
	"github.com/bitcoinsv/bsvutil"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
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

	logger.Info("mining info", "blocks", info.Blocks, "current block size", info.CurrentBlockSize)

	walletName := "test-1"
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

	// privKey, err := btcec.NewPrivateKey()
	// if err != nil {
	// 	return err
	// }

	// fmt.Println(hex.EncodeToString(privKey.Serialize()))

	privKeyBytes, err := hex.DecodeString("2b6806f4f1b9dc64ef42199d9d48e1a3ce4d8562271065526a3975c3ae86a02f")
	if err != nil {
		return err
	}
	privKey, pubKey := btcec.PrivKeyFromBytes(privKeyBytes)
	address, err := btcutil.NewAddressPubKey(pubKey.SerializeCompressed(),
		&chaincfg.RegressionNetParams)
	if err != nil {
		return err
	}

	addr, err := client.GetNewAddress(walletName)
	if err != nil {
		return err
	}
	// fmt.Println(addr.EncodeAddress())

	// address, err := btcutil.NewAddressPubKey(privKey.PubKey().SerializeUncompressed(), &chaincfg.RegressionNetParams)
	// if err != nil {
	// 	return err
	// }

	logger.Info("address", "address", address.EncodeAddress())
	logger.Info("wallet address", "address", addr.EncodeAddress())

	// accountName := "test-account"
	// err = client.SetAccount(address, accountName)
	// if err != nil {
	// 	return nil
	// }

	// _, err = client.DumpPrivKey(address)
	// if err != nil {
	// 	return nil
	// }

	// // privateKey, err := client.DumpWallet(address)
	// if err != nil {
	// 	return fmt.Errorf("failed to dump priv key: %v", err)
	// }

	// logger.Info("new private key", "key", privateKey.String())

	// hashes, err := client.GenerateToAddress(1, addr, nil)
	// if err != nil {
	// 	return fmt.Errorf("failed to gnereate to address: %v", err)
	// }
	// for _, hash := range hashes {
	// 	logger.Info("hash", "hex string", hash.String())
	// }

	// unspent, err := client.ListUnspentMinMax(0, 99999)
	unspent, err := client.ListUnspent()
	if err != nil {
		return fmt.Errorf("failed to list received by address: %v", err)
	}
	for _, u := range unspent {
		logger.Info("unspent", "TxID", u.TxID, "address", u.Address, "amount", u.Amount)
	}

	hash, err := chainhash.NewHashFromStr(unspent[0].TxID)
	if err != nil {
		return err
	}

	rawTx, err := client.GetRawTransaction(hash)
	if err != nil {
		return err
	}

	tx, err := utils.PayToAddress(address, rawTx, privKey)
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

func transformUTXOs(utxos []btcjson.ListUnspentResult, filterAccountName string) []NodeUnspentUtxo {
	outputs := make([]NodeUnspentUtxo, 0)

	for _, utxo := range utxos {

		if utxo.Account != filterAccountName {
			continue
		}

		outputs = append(outputs, NodeUnspentUtxo{
			Txid:         utxo.TxID,
			Vout:         utxo.Vout,
			ScriptPubKey: utxo.ScriptPubKey,
			Amount:       utxo.Amount,
		})
	}

	return outputs
}

func createTxFrom(privateKey string, address string, utxos []NodeUnspentUtxo, fee ...uint64) (*sdkTx.Transaction, error) {
	tx := sdkTx.NewTransaction()

	// Add an input using the UTXOs
	for _, utxo := range utxos {
		utxoTxID := utxo.Txid
		utxoVout := utxo.Vout
		utxoSatoshis := uint64(utxo.Amount * 1e8) // Convert BTC to satoshis
		utxoScript := utxo.ScriptPubKey

		u, err := sdkTx.NewUTXO(utxoTxID, utxoVout, utxoScript, utxoSatoshis)
		if err != nil {
			return nil, fmt.Errorf("failed creating UTXO: %v", err)
		}
		err = tx.AddInputsFromUTXOs(u)
		if err != nil {
			return nil, fmt.Errorf("failed adding input: %v", err)
		}
	}
	// Add an output to the address you've previously created
	recipientAddress := address

	var feeValue uint64
	if len(fee) > 0 {
		feeValue = fee[0]
	} else {
		feeValue = 20 // Set your default fee value here
	}

	total, err := tx.TotalInputSatoshis()
	if err != nil {
		return nil, err
	}

	amountToSend := total - feeValue

	err = tx.PayToAddress(recipientAddress, amountToSend)
	if err != nil {
		return nil, fmt.Errorf("failed to pay to address: %v", err)
	}

	// Sign the input
	wif, err := bsvutil.DecodeWIF(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode WIF: %v", err)
	}

	// Extract raw private key bytes directly from the WIF structure
	privateKeyDecoded := wif.PrivKey.Serialize()
	pk, _ := ec.PrivateKeyFromBytes(privateKeyDecoded)

	unlockingScriptTemplate, err := p2pkh.Unlock(pk, nil)
	if err != nil {
		return nil, err
	}

	for _, input := range tx.Inputs {
		input.UnlockingScriptTemplate = unlockingScriptTemplate
	}

	err = tx.Sign()
	if err != nil {
		return nil, err
	}

	return tx, nil
}

type NodeUnspentUtxo struct {
	Txid         string
	Vout         uint32
	Amount       float64
	ScriptPubKey string
}
