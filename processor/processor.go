package processor

import (
	"errors"
	"fmt"
	"log/slog"
	"node-analysis/utils"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
)

type Processor struct {
	client  *rpcclient.Client
	address btcutil.Address
	logger  *slog.Logger
	privKey *btcec.PrivateKey
}

var (
	ErrOutputAlreadySpent = errors.New("output already spent")
)

const targetUtxos = 10000
const outputsPerTx = 100

func New(client *rpcclient.Client, logger *slog.Logger, address btcutil.Address, privKey *btcec.PrivateKey) *Processor {

	p := &Processor{
		client:  client,
		logger:  logger,
		address: address,
		privKey: privKey,
	}

	return p
}

func (p *Processor) GetTxOutHashFromBlock(blockHash *chainhash.Hash) (utils.TxOut, error) {
	lastBlock, err := p.client.GetBlock(blockHash)
	if err != nil {
		return utils.TxOut{}, err
	}

	txHash := lastBlock.Transactions[0].TxHash()

	// USE GETTXOUT https://bitcoin.stackexchange.com/questions/117919/bitcoin-cli-listunspent-returns-empty-list
	txOut, err := p.client.GetTxOut(&txHash, 0, false)
	if err != nil {
		return utils.TxOut{}, err
	}

	if txOut == nil {
		return utils.TxOut{}, ErrOutputAlreadySpent
	}

	return utils.TxOut{
		Hash:            &txHash,
		ValueSat:        int64(txOut.Value * 1e8),
		ScriptPubKeyHex: txOut.ScriptPubKey.Hex,
	}, nil
}

func (p *Processor) PrepareUtxos() error {

	info, err := p.client.GetMiningInfo()
	if err != nil {
		return fmt.Errorf("failed to get info: %v", err)
	}

	if info.Blocks <= 100 {

		blocksToGenerate := 101 - info.Blocks

		_, err := p.client.GenerateToAddress(blocksToGenerate, p.address, nil)
		if err != nil {
			return fmt.Errorf("failed to gnereate to address: %v", err)
		}
	}

	blockHash, err := p.client.GetBlockHash(info.Blocks - 100)
	if err != nil {
		return err
	}

	txOut, err := p.GetTxOutHashFromBlock(blockHash)
	if err != nil {
		return err
	}

	counter := 0
	for err == ErrOutputAlreadySpent {

		if counter > 20 {
			return errors.New("too many outputs already spent")
		}
		_, err = p.client.GenerateToAddress(1, p.address, nil)
		if err != nil {
			return fmt.Errorf("failed to gnereate to address: %v", err)
		}

		info, err = p.client.GetMiningInfo()
		if err != nil {
			return fmt.Errorf("failed to get info: %v", err)
		}

		blockHash, err := p.client.GetBlockHash(info.Blocks - 100)
		if err != nil {
			return err
		}
		txOut, err = p.GetTxOutHashFromBlock(blockHash)
		if err != nil {
			return err
		}
	}

	p.logger.Info("tx", "hash", txOut.Hash.String(), "value", txOut.ValueSat, "blockhash", blockHash.String())

	tx, err := utils.SplitToAddress(p.address, txOut, outputsPerTx, p.privKey)
	if err != nil {
		return err
	}

	sentTxHash, err := p.client.SendRawTransaction(tx, false)
	if err != nil {
		return err
	}

	p.logger.Info("sent raw tx", "hash", sentTxHash.String())

	return nil

}
