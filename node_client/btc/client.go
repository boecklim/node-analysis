package btc

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"node-analysis/broadcaster"
	"os"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	coinBaseVout               = 0
	satPerBtc                  = 1e8
	coinbaseSpendableAfterConf = 100
	targetUtxos                = 150
	outputsPerTx               = 20 // must be lower than 25 otherwise err="-26: too-long-mempool-chain, too many descendants for tx ..."
	fee                        = 3000
)

var _ broadcaster.RPCClient = &Client{}

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

func New(client *rpcclient.Client) (*Client, error) {
	p := &Client{
		client: client,
		logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	err := p.setAddress()
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Client) getCoinbaseTxOutFromBlock(blockHash *chainhash.Hash) (broadcaster.TxOut, error) {
	lastBlock, err := p.client.GetBlock(blockHash)
	if err != nil {
		return broadcaster.TxOut{}, err
	}

	txHash := lastBlock.Transactions[0].TxHash()

	// USE GETTXOUT https://bitcoin.stackexchange.com/questions/117919/bitcoin-cli-listunspent-returns-empty-list
	txOut, err := p.client.GetTxOut(&txHash, coinBaseVout, false)
	if err != nil {
		return broadcaster.TxOut{}, err
	}

	if txOut == nil {
		return broadcaster.TxOut{}, ErrOutputSpent
	}

	return broadcaster.TxOut{
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

func (p *Client) SubmitSelfPayingSingleOutputTx(txOut broadcaster.TxOut) (txHash *chainhash.Hash, satoshis int64, err error) {
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

func (p *Client) createSelfPayingTx(txOut broadcaster.TxOut) (*wire.MsgTx, error) {
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

func (p *Client) setAddress() error {
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

	p.address = address
	p.privKey = privKey

	p.logger.Info("address", "address", address.EncodeAddress())

	pkScript, err := txscript.PayToAddrScript(p.address)
	if err != nil {
		return err
	}

	p.pkScript = pkScript
	return nil
}

func (p *Client) PrepareUtxos(utxoChannel chan broadcaster.TxOut) error {
	blocks, err := p.getBlocks()
	if err != nil {
		return fmt.Errorf("failed to get info: %v", err)
	}

	if blocks <= coinbaseSpendableAfterConf {
		blocksToGenerate := coinbaseSpendableAfterConf + 1 - blocks
		p.logger.Info("generating blocks", "number", blocksToGenerate)

		_, err = p.client.GenerateToAddress(blocksToGenerate, p.address, nil)
		if err != nil {
			return fmt.Errorf("failed to gnereate to address: %v", err)
		}
	}

	for len(utxoChannel) < targetUtxos {
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

		var txOut broadcaster.TxOut
		txOut, err = p.getCoinbaseTxOutFromBlock(blockHash)
		if err != nil {
			if errors.Is(err, ErrOutputSpent) {
				continue
			}
			return err
		}

		p.logger.Info("splittable output", "hash", txOut.Hash.String(), "value", txOut.ValueSat, "blockhash", blockHash.String())

		var tx *wire.MsgTx
		tx, err = splitToAddress(p.address, txOut, outputsPerTx, p.privKey, fee)
		if err != nil {
			return fmt.Errorf("failed split to address: %v", err)
		}

		var sentTxHash *chainhash.Hash
		sentTxHash, err = p.client.SendRawTransaction(tx, false)
		if err != nil {
			return fmt.Errorf("failed to send raw tx: %v", err)
		}

		p.logger.Info("sent raw tx", "hash", sentTxHash.String(), "outputs", len(tx.TxOut))

		for i, output := range tx.TxOut {
			utxoChannel <- broadcaster.TxOut{
				Hash:            sentTxHash,
				ScriptPubKeyHex: hex.EncodeToString(output.PkScript),
				ValueSat:        output.Value,
				VOut:            uint32(i),
			}
		}
	}

	return nil
}

func splitToAddress(address btcutil.Address, txOut broadcaster.TxOut, outputs int, privKey *btcec.PrivateKey, fee int64) (*wire.MsgTx, error) {
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
