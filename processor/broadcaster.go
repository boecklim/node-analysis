package processor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

type RPCClient interface {
	PrepareUtxos(utxoChannel chan TxOut, targetUtxos int) (blockHashes map[string]struct{}, err error)
	SubmitSelfPayingSingleOutputTx(txOut TxOut) (txHash *chainhash.Hash, satoshis int64, err error)
	GenerateBlock() (blockID string, err error)
	GetBlockSize(blockHash *chainhash.Hash) (sizeBytes uint64, nrTxs uint64, err error)
}

type Broadcaster struct {
	client           RPCClient
	addressScriptHex string
	logger           *slog.Logger
	utxoChannel      chan TxOut

	cancelAll context.CancelFunc
	ctx       context.Context
	shutdown  chan struct{}
	wg        sync.WaitGroup
	totalTxs  int64
	limit     int64
	txChannel chan *wire.MsgTx
}

const (
	millisecondsPerSecond = 1000
)

func NewBroadcaster(client RPCClient, logger *slog.Logger) (*Broadcaster, error) {
	b := &Broadcaster{
		client:      client,
		logger:      logger,
		utxoChannel: make(chan TxOut, 10100),
		shutdown:    make(chan struct{}, 1),
		txChannel:   make(chan *wire.MsgTx, 10100),
	}

	ctx, cancelAll := context.WithCancel(context.Background())
	b.cancelAll = cancelAll
	b.ctx = ctx

	return b, nil
}

func (b *Broadcaster) PrepareUtxos(targetUtxos int) (blockHahses map[string]struct{}, err error) {
	blockHashes, err := b.client.PrepareUtxos(b.utxoChannel, targetUtxos)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare utxos: %v", err)
	}

	return blockHashes, nil
}

func (b *Broadcaster) Start(rateTxsPerSecond int64, limit int64) (err error) {
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
	var satoshis int64
	var hash *chainhash.Hash
	statTicker := time.NewTicker(5 * time.Second)

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
			case <-statTicker.C:
				b.logger.Info("stats", slog.Int64("total", atomic.LoadInt64(&b.totalTxs)), slog.Int64("limit", b.limit), slog.Int("utxos", len(b.utxoChannel)))
			case <-submitTicker.C:

				if b.limit > 0 && atomic.LoadInt64(&b.totalTxs) >= b.limit {
					b.logger.Info("limit reached", slog.Int64("total", atomic.LoadInt64(&b.totalTxs)), slog.Int64("limit", b.limit))
					b.shutdown <- struct{}{}
				}

				txOut := <-b.utxoChannel

				hash, satoshis, err = b.client.SubmitSelfPayingSingleOutputTx(txOut)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}

					b.logger.Error("submitting tx failed", "hash", txOut.Hash.String())

					b.utxoChannel <- TxOut{
						Hash:            hash,
						ScriptPubKeyHex: b.addressScriptHex,
						ValueSat:        satoshis,
						VOut:            0,
					}

					errCh <- err
					continue
				}

				b.logger.Debug("submitting tx successful", "hash", hash.String())
				b.utxoChannel <- TxOut{
					Hash:            hash,
					ScriptPubKeyHex: b.addressScriptHex,
					ValueSat:        satoshis,
					VOut:            0,
				}

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

type TxOut struct {
	Hash            *chainhash.Hash
	ScriptPubKeyHex string
	ValueSat        int64
	VOut            uint32
}
