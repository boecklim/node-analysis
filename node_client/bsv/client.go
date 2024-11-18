package bsv

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"node-analysis/broadcaster"
	"node-analysis/utils"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"

	"github.com/ordishs/go-bitcoin"
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
	client *bitcoin.Bitcoind
}

func New(client *bitcoin.Bitcoind) (*Client, error) {

	p := &Client{
		client: client,
	}

	return p, nil
}

func (p *Client) GetCoinbaseTxOutFromBlock(blockHash *chainhash.Hash) (utils.TxOut, error) {
	lastBlock, err := p.client.GetBlock(blockHash.String())
	if err != nil {
		return utils.TxOut{}, err
	}

	txHash := lastBlock.CoinbaseTx.Hash

	// USE GETTXOUT https://bitcoin.stackexchange.com/questions/117919/bitcoin-cli-listunspent-returns-empty-list
	txOut, err := p.client.GetTxOut(txHash, coinBaseVout, false)
	if err != nil {
		return utils.TxOut{}, err
	}

	if txOut == nil {
		return utils.TxOut{}, ErrOutputSpent
	}

	hash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return utils.TxOut{}, err
	}

	return utils.TxOut{
		Hash:            hash,
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

	return int64(info.Blocks), nil

}

func (p *Client) GenerateToAddress(numBlocks int64, address btcutil.Address) error {
	_, err := p.client.GenerateToAddress(1, address.EncodeAddress())
	if err != nil {
		return fmt.Errorf("failed to gnereate to address: %v", err)
	}

	return nil
}

func (p *Client) GetBlockHash(blockHeight int64) (*chainhash.Hash, error) {
	blockHash, err := p.client.GetBlockHash(int(blockHeight) - coinbaseSpendableAfterConf)
	if err != nil {
		return nil, err
	}
	hash, err := chainhash.NewHashFromStr(blockHash)
	if err != nil {
		return nil, err
	}

	return hash, nil
}

func (p *Client) SendRawTransaction(tx *wire.MsgTx) (*chainhash.Hash, error) {

	// Serialize the transaction and convert to hex string.
	buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
	err := tx.Serialize(buf)
	if err != nil {
		return nil, err
	}

	txHex := hex.EncodeToString(buf.Bytes())

	sentTxHash, err := p.client.SendRawTransaction(txHex)
	if err != nil {
		return nil, err
	}

	hash, err := chainhash.NewHashFromStr(sentTxHash)
	if err != nil {
		return nil, err
	}

	return hash, nil
}
