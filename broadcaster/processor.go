package broadcaster

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"node-analysis/utils"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

type RPCClient interface {
	GetCoinbaseTxOutFromBlock(blockHash *chainhash.Hash) (utils.TxOut, error)
	GetBlocks() (int64, error)
	GenerateToAddress(numBlocks int64, address btcutil.Address) error
	GetBlockHash(blockHeight int64) (*chainhash.Hash, error)
	SendRawTransaction(tx *wire.MsgTx) (*chainhash.Hash, error)
}

type Broadcaster struct {
	client           RPCClient
	address          btcutil.Address
	addressScriptHex string
	pkScript         []byte
	logger           *slog.Logger
	privKey          *btcec.PrivateKey
	utxoChannel      chan utils.TxOut

	cancelAll  context.CancelFunc
	ctx        context.Context
	shutdown   chan struct{}
	wg         sync.WaitGroup
	totalTxs   int64
	limit      int64
	satoshiMap sync.Map
	txChannel  chan *wire.MsgTx
}

var (
	ErrOutputSpent = errors.New("output already spent")
)

const (
	targetUtxos                = 150
	outputsPerTx               = 20 // must be lower than 25 other wise err="-26: too-long-mempool-chain, too many descendants for tx ..."
	coinBaseVout               = 0
	satPerBtc                  = 1e8
	coinbaseSpendableAfterConf = 100
	millisecondsPerSecond      = 1000
	fee                        = 3000
)

func New(client RPCClient, logger *slog.Logger, address btcutil.Address, privKey *btcec.PrivateKey) (*Broadcaster, error) {

	pkScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return nil, err
	}
	p := &Broadcaster{
		client:           client,
		logger:           logger,
		address:          address,
		addressScriptHex: hex.EncodeToString(address.ScriptAddress()),
		pkScript:         pkScript,
		privKey:          privKey,
		utxoChannel:      make(chan utils.TxOut, 10100),
		shutdown:         make(chan struct{}, 1),
		txChannel:        make(chan *wire.MsgTx, 10100),
	}

	ctx, cancelAll := context.WithCancel(context.Background())
	p.cancelAll = cancelAll
	p.ctx = ctx

	return p, nil
}

func (p *Broadcaster) PrepareUtxos() error {

	blocks, err := p.client.GetBlocks()
	if err != nil {
		return fmt.Errorf("failed to get info: %v", err)
	}

	if blocks <= coinbaseSpendableAfterConf {

		blocksToGenerate := coinbaseSpendableAfterConf + 1 - blocks
		p.logger.Info("generating blocks", "number", blocksToGenerate)

		err := p.client.GenerateToAddress(blocksToGenerate, p.address)
		if err != nil {
			return fmt.Errorf("failed to gnereate to address: %v", err)
		}
	}

	for len(p.utxoChannel) < targetUtxos {

		err := p.client.GenerateToAddress(1, p.address)
		if err != nil {
			return fmt.Errorf("failed to gnereate to address: %v", err)
		}

		blocks, err = p.client.GetBlocks()
		if err != nil {
			return fmt.Errorf("failed to get info: %v", err)
		}

		blockHash, err := p.client.GetBlockHash(blocks - coinbaseSpendableAfterConf)
		if err != nil {
			return err
		}

		txOut, err := p.client.GetCoinbaseTxOutFromBlock(blockHash)
		if err != nil {
			if errors.Is(err, ErrOutputSpent) {
				continue
			}
			return err
		}

		p.logger.Info("splittable output", "hash", txOut.Hash.String(), "value", txOut.ValueSat, "blockhash", blockHash.String())

		tx, err := utils.SplitToAddress(p.address, txOut, outputsPerTx, p.privKey, fee)
		if err != nil {
			return err
		}

		sentTxHash, err := p.client.SendRawTransaction(tx)
		if err != nil {
			return err
		}

		p.logger.Info("sent raw tx", "hash", sentTxHash.String(), "outputs", len(tx.TxOut))

		for i, output := range tx.TxOut {

			txOut := utils.TxOut{
				Hash:            sentTxHash,
				ScriptPubKeyHex: hex.EncodeToString(output.PkScript),
				ValueSat:        output.Value,
				VOut:            uint32(i),
			}

			p.utxoChannel <- txOut
		}
	}

	return nil

}
