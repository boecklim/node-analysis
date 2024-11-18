package btc

import (
	"errors"
	"fmt"
	"node-analysis/broadcaster"
	"node-analysis/utils"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
)

const (
	coinBaseVout               = 0
	satPerBtc                  = 1e8
	coinbaseSpendableAfterConf = 100
)

var _ broadcaster.RPCClient = &Client{}

var (
	ErrOutputSpent = errors.New("output already spent")
)

type Client struct {
	client *rpcclient.Client
}

func New(client *rpcclient.Client) (*Client, error) {

	p := &Client{
		client: client,
	}

	return p, nil
}

func (p *Client) GetCoinbaseTxOutFromBlock(blockHash *chainhash.Hash) (utils.TxOut, error) {
	lastBlock, err := p.client.GetBlock(blockHash)
	if err != nil {
		return utils.TxOut{}, err
	}

	txHash := lastBlock.Transactions[0].TxHash()

	// USE GETTXOUT https://bitcoin.stackexchange.com/questions/117919/bitcoin-cli-listunspent-returns-empty-list
	txOut, err := p.client.GetTxOut(&txHash, coinBaseVout, false)
	if err != nil {
		return utils.TxOut{}, err
	}

	if txOut == nil {
		return utils.TxOut{}, ErrOutputSpent
	}

	return utils.TxOut{
		Hash:            &txHash,
		ValueSat:        int64(txOut.Value * satPerBtc),
		ScriptPubKeyHex: txOut.ScriptPubKey.Hex,
		VOut:            0,
	}, nil
}

func (p *Client) GetBlocks() (int64, error) {
	info, err := p.client.GetMiningInfo()
	if err != nil {
		return 0, fmt.Errorf("failed to get info: %v", err)
	}

	return info.Blocks, nil

}

func (p *Client) GenerateToAddress(numBlocks int64, address btcutil.Address) error {
	_, err := p.client.GenerateToAddress(1, address, nil)
	if err != nil {
		return fmt.Errorf("failed to gnereate to address: %v", err)
	}

	return nil
}

func (p *Client) GetBlockHash(blockHeight int64) (*chainhash.Hash, error) {
	blockHash, err := p.client.GetBlockHash(blockHeight - coinbaseSpendableAfterConf)
	if err != nil {
		return nil, err
	}

	return blockHash, nil
}

func (p *Client) SendRawTransaction(tx *wire.MsgTx) (*chainhash.Hash, error) {
	sentTxHash, err := p.client.SendRawTransaction(tx, false)
	if err != nil {
		return nil, err
	}

	return sentTxHash, nil
}
