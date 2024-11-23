package broadcaster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"node-analysis/utils"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

type RPCClient interface {
	PrepareUtxos(utxoChannel chan utils.TxOut) error
	SubmitSelfPayingSingleOutputTx(txOut utils.TxOut) (txHash *chainhash.Hash, satoshis int64, err error)
}

type Broadcaster struct {
	client           RPCClient
	addressScriptHex string
	logger           *slog.Logger
	utxoChannel      chan utils.TxOut

	cancelAll context.CancelFunc
	ctx       context.Context
	shutdown  chan struct{}
	wg        sync.WaitGroup
	totalTxs  int64
	limit     int64
	txChannel chan *wire.MsgTx
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

func New(client RPCClient, logger *slog.Logger) (*Broadcaster, error) {
	b := &Broadcaster{
		client:      client,
		logger:      logger,
		utxoChannel: make(chan utils.TxOut, 10100),
		shutdown:    make(chan struct{}, 1),
		txChannel:   make(chan *wire.MsgTx, 10100),
	}

	ctx, cancelAll := context.WithCancel(context.Background())
	b.cancelAll = cancelAll
	b.ctx = ctx

	return b, nil
}

func (b *Broadcaster) PrepareUtxos() error {
	err := b.client.PrepareUtxos(b.utxoChannel)
	if err != nil {
		return fmt.Errorf("failed to prepare utxos: %v", err)
	}

	return nil
}

func (b *Broadcaster) Start(rateTxsPerSecond int64, limit int64) error {
	b.limit = limit
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case <-b.shutdown:
				b.cancelAll()
			case <-b.ctx.Done():
				return
			}
		}
	}()

	b.logger.Info("Start broadcasting", "outputs", len(b.utxoChannel))

	submitInterval := time.Duration(millisecondsPerSecond/float64(rateTxsPerSecond)) * time.Millisecond
	submitTicker := time.NewTicker(submitInterval)

	errCh := make(chan error, 100)

	b.wg.Add(1)
	go func() {
		defer func() {
			b.logger.Info("stopping broadcasting")
			b.wg.Done()
		}()

		for {
			select {
			case <-b.ctx.Done():
				return
			case <-submitTicker.C:

				if b.limit > 0 && atomic.LoadInt64(&b.totalTxs) >= b.limit {
					b.logger.Info("limit reached", slog.Int64("total", atomic.LoadInt64(&b.totalTxs)), slog.Int64("limit", b.limit))
					b.shutdown <- struct{}{}
				}

				txOut := <-b.utxoChannel

				hash, satoshis, err := b.client.SubmitSelfPayingSingleOutputTx(txOut)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}

					b.logger.Error("submitting tx failed", "hash", txOut.Hash.String())

					b.utxoChannel <- utils.TxOut{
						Hash:            hash,
						ScriptPubKeyHex: b.addressScriptHex,
						ValueSat:        satoshis,
						VOut:            0,
					}

					errCh <- err
					continue
				}

				b.logger.Debug("submitting tx successful", "hash", hash.String())
				newUtxo := utils.TxOut{
					Hash:            hash,
					ScriptPubKeyHex: b.addressScriptHex,
					ValueSat:        satoshis,
					VOut:            0,
				}
				b.utxoChannel <- newUtxo

				atomic.AddInt64(&b.totalTxs, 1)

			case responseErr := <-errCh:
				b.logger.Error("failed to submit transactions", slog.String("err", responseErr.Error()))
			}
		}
	}()

	b.wg.Wait()

	return nil
}

func (b *Broadcaster) Shutdown() {
	b.cancelAll()

	b.wg.Wait()
}
