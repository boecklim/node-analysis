package btc

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/boecklim/node-analysis/processor"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/boecklim/node-analysis/node_client/btc/rpcclient"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	coinBaseVout               = 0
	satPerBtc                  = 1e8
	coinbaseSpendableAfterConf = 100
	outputsPerTx               = 20 // must be lower than 25 otherwise err="-26: too-long-mempool-chain, too many descendants for tx ..."
	fee                        = 3000
)

var _ processor.RPCClient = &Client{}

var (
	ErrOutputSpent = errors.New("output already spent")
)

type Client struct {
	client *rpcclient.Client
	logger *slog.Logger

	pkScript []byte
	address  btcutil.Address
	privKey  *btcec.PrivateKey
}

func New(client *rpcclient.Client, logger *slog.Logger) (*Client, error) {
	p := &Client{
		client: client,
		logger: logger,
	}

	err := p.setAddress()
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Client) setAddress() error {
	var err error
	var privKey *btcec.PrivateKey

	privKey, err = btcec.NewPrivateKey()
	if err != nil {
		return fmt.Errorf("failed to create private key: %w", err)
	}

	address, err := btcutil.NewAddressPubKey(privKey.PubKey().SerializeCompressed(),
		&chaincfg.RegressionNetParams)
	if err != nil {
		return err
	}

	p.address = address
	p.privKey = privKey

	p.logger.Info("New address", "address", address.EncodeAddress())

	pkScript, err := txscript.PayToAddrScript(p.address)
	if err != nil {
		return err
	}

	p.pkScript = pkScript
	return nil
}

func (p *Client) getCoinbaseTxOutFromBlock(blockHash *chainhash.Hash) (processor.TxOut, error) {
	lastBlock, err := p.client.GetBlock(blockHash)
	if err != nil {
		return processor.TxOut{}, err
	}

	txHash := lastBlock.Transactions[0].TxHash()

	// USE GETTXOUT https://bitcoin.stackexchange.com/questions/117919/bitcoin-cli-listunspent-returns-empty-list
	txOut, err := p.client.GetTxOut(&txHash, coinBaseVout, false)
	if err != nil {
		return processor.TxOut{}, err
	}

	if txOut == nil {
		return processor.TxOut{}, ErrOutputSpent
	}

	return processor.TxOut{
		Hash:            &txHash,
		ValueSat:        int64(txOut.Value * satPerBtc),
		ScriptPubKeyHex: txOut.ScriptPubKey.Hex,
		VOut:            0,
	}, nil
}

func (p *Client) getBlocks() (int64, error) {
	info, err := p.client.GetMiningInfo()
	if err != nil {
		return 0, fmt.Errorf("failed to get info: %v", err)
	}

	return info.Blocks, nil
}

func (p *Client) SubmitSelfPayingSingleOutputTx(txOut processor.TxOut) (txHash *chainhash.Hash, satoshis int64, err error) {
	tx, err := p.createSelfPayingTx(txOut)
	if err != nil {
		return nil, 0, err
	}

	txHash, err = p.client.SendRawTransaction(tx, false)
	if err != nil {
		return nil, 0, err
	}

	return txHash, tx.TxOut[0].Value, nil
}

func (p *Client) createSelfPayingTx(txOut processor.TxOut) (*wire.MsgTx, error) {
	if txOut.Hash == nil {
		return nil, fmt.Errorf("hash is missing")
	}

	p.logger.Debug("creating tx", "prev tx hash", txOut.Hash.String(), "vout", txOut.VOut)

	tx := wire.NewMsgTx(wire.TxVersion)
	amount := txOut.ValueSat

	prevOut := wire.NewOutPoint(txOut.Hash, txOut.VOut)
	input := wire.NewTxIn(prevOut, nil, nil)

	tx.AddTxIn(input)

	amount -= fee

	tx.AddTxOut(wire.NewTxOut(amount, []byte(p.pkScript)))

	lookupKey := func(_ btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return p.privKey, true, nil
	}
	sigScript, err := txscript.SignTxOutput(&chaincfg.MainNetParams,
		tx, 0, p.pkScript, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	p.logger.Debug("tx created", "hash", tx.TxID())

	return tx, nil
}

func (p *Client) GetBlockSize(blockHash *chainhash.Hash) (sizeBytes uint64, nrTxs uint64, err error) {
	blockMsg, err := p.client.GetBlock(blockHash)
	if err != nil {
		return 0, 0, err
	}

	return uint64(blockMsg.SerializeSize()), uint64(len(blockMsg.Transactions)), nil
}

func (p *Client) PrepareUtxos(utxoChannel chan processor.TxOut, targetUtxos int) (err error) {
	blocks, err := p.getBlocks()
	if err != nil {
		return fmt.Errorf("failed to get info: %v", err)
	}

	signalFinish := make(chan struct{})
	loggingStopped := make(chan struct{})
	showTicker := time.NewTicker(2 * time.Second)
	go func() {
		defer close(loggingStopped)

		for {
			select {
			case <-showTicker.C:
				p.logger.Info("Creating utxos", slog.Int("count", len(utxoChannel)), slog.Int("target", targetUtxos))
			case <-signalFinish:
				return
			}
		}
	}()

	if blocks <= coinbaseSpendableAfterConf {
		blocksToGenerate := coinbaseSpendableAfterConf + 1 - blocks
		p.logger.Info("Generating blocks", "number", blocksToGenerate)

		_, err = p.client.GenerateToAddress(blocksToGenerate, p.address, nil)
		if err != nil {
			return fmt.Errorf("failed to gnereate to address: %v", err)
		}
	}

	counter := 0

	for len(utxoChannel) < targetUtxos {

		// Todo: Do not generate block at each iteration. Only generate new block if err chain of txs too long
		_, err = p.client.GenerateToAddress(1, p.address, nil)
		if err != nil {
			return fmt.Errorf("failed to gnereate to address: %v", err)
		}
		blocks, err = p.getBlocks()
		if err != nil {
			return fmt.Errorf("failed to get info: %v", err)
		}

		blockHeight := blocks - coinbaseSpendableAfterConf
		var blockHash *chainhash.Hash
		blockHash, err = p.client.GetBlockHash(blockHeight)
		if err != nil {
			return fmt.Errorf("failed go get block hash at height %d: %v", blockHeight, err)
		}

		var txOut processor.TxOut
		txOut, err = p.getCoinbaseTxOutFromBlock(blockHash)
		if err != nil {
			if errors.Is(err, ErrOutputSpent) {
				continue
			}
			return err
		}

		p.logger.Debug("Splittable output", "hash", txOut.Hash.String(), "value", txOut.ValueSat, "blockhash", blockHash.String())

		var tx *wire.MsgTx
		tx, err = splitToAddress(p.address, txOut, outputsPerTx, p.privKey, fee)
		if err != nil {
			return fmt.Errorf("failed split to address: %v", err)
		}

		var sentTxHash *chainhash.Hash
		sentTxHash, err = p.client.SendRawTransaction(tx, false)
		if err != nil {
			if strings.Contains(err.Error(), "Transaction outputs already in utxo set") && counter < 5 {
				counter++
				p.logger.Warn("Failed to send raw tx", slog.String("err", err.Error()))
				continue
			}
			return fmt.Errorf("failed to send raw tx: %v", err)
		}

		p.logger.Debug("Sent raw tx", "hash", sentTxHash.String(), "outputs", len(tx.TxOut))

		for i, output := range tx.TxOut {
			utxoChannel <- processor.TxOut{
				Hash:            sentTxHash,
				ScriptPubKeyHex: hex.EncodeToString(output.PkScript),
				ValueSat:        output.Value,
				VOut:            uint32(i),
			}
		}
	}

	_, err = p.client.GenerateToAddress(1, p.address, nil)
	if err != nil {
		return fmt.Errorf("failed to gnereate to address: %v", err)
	}

	close(signalFinish)
	<-loggingStopped
	p.logger.Info("Created utxos", slog.Int("count", len(utxoChannel)), slog.Int("target", targetUtxos))

	return nil
}

func splitToAddress(address btcutil.Address, txOut processor.TxOut, outputs int, privKey *btcec.PrivateKey, fee int64) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	prevOut := wire.NewOutPoint(txOut.Hash, 0)
	input := wire.NewTxIn(prevOut, nil, nil)
	tx.AddTxIn(input)

	pkScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return nil, err
	}

	remainingSat := txOut.ValueSat

	satPerOutput := int64(math.Floor(float64(txOut.ValueSat) / float64(outputs+1)))

	for range outputs {
		tx.AddTxOut(wire.NewTxOut(satPerOutput, pkScript))
		remainingSat -= satPerOutput
	}

	tx.AddTxOut(wire.NewTxOut(remainingSat-fee, pkScript))

	lookupKey := func(_ btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return privKey, true, nil
	}

	pkScriptOrig, err := hex.DecodeString(txOut.ScriptPubKeyHex)
	if err != nil {
		return nil, err
	}

	sigScript, err := txscript.SignTxOutput(&chaincfg.MainNetParams,
		tx, 0, pkScriptOrig, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	return tx, nil
}

func (p *Client) GenerateBlock() (blockID string, err error) {
	blockHash, err := p.client.GenerateToAddress(1, p.address, nil)
	if err != nil {
		return "", err
	}

	return blockHash[0].String(), nil
}
