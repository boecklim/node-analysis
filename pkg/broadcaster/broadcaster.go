package broadcaster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type Processor interface {
	PrepareUtxos(utxoChannel chan TxOut, targetUtxos int) (err error)
	SubmitSelfPayingSingleOutputTx(txOut TxOut) (txHash *chainhash.Hash, satoshis int64, err error)
	GetMempoolSize() (nrTxs uint64, err error)
}

type Broadcaster struct {
	processor   Processor
	utxoChannel chan TxOut

	cancelAll context.CancelFunc
	ctx       context.Context
	wg        sync.WaitGroup
	totalTxs  int64
	limit     time.Duration
}

const (
	millisecondsPerSecond = 1000
)

func NewBroadcaster(client Processor) (*Broadcaster, error) {
	b := &Broadcaster{
		processor:   client,
		utxoChannel: make(chan TxOut, 10100),
	}

	ctx, cancelAll := context.WithCancel(context.Background())
	b.cancelAll = cancelAll
	b.ctx = ctx

	return b, nil
}

func (b *Broadcaster) PrepareUtxos(targetUtxos int) (err error) {
	err = b.processor.PrepareUtxos(b.utxoChannel, targetUtxos)
	if err != nil {
		return fmt.Errorf("failed to prepare utxos: %v", err)
	}

	return nil
}

func (b *Broadcaster) Start(rateTxsPerSecond int64, limit time.Duration, logger *slog.Logger, startAt time.Time) (err error) {
	b.limit = limit
	deadline := time.Now().Add(limit)

	logger = logger.With(slog.String("service", "broadcaster"))

	startTimer := time.NewTimer(time.Until(startAt))
	logger.Info("Waiting to start", "until", startAt.String())
	<-startTimer.C

	logger.Info("Starting broadcasting", "outputs", len(b.utxoChannel))

	submitInterval := time.Duration(millisecondsPerSecond/float64(rateTxsPerSecond)) * time.Millisecond
	submitTicker := time.NewTicker(submitInterval)

	var satoshis int64
	var hash *chainhash.Hash
	statTicker := time.NewTicker(5 * time.Second)
	ctx, cancel := context.WithDeadline(b.ctx, deadline)
	defer cancel()

	b.wg.Add(1)
	go func() {
		defer func() {
			logger.Info("Stopping broadcasting")
			b.wg.Done()
		}()

	mainLoop:
		for {
			select {
			case <-ctx.Done():
				return
			case <-statTicker.C:
				var mempoolSize uint64
				mempoolSize, err = b.processor.GetMempoolSize()
				if err != nil {
					logger.Error("Failed to get mempool size", "err", err)
				}

				logger.Info("Stats", slog.Int64("total", atomic.LoadInt64(&b.totalTxs)), slog.String("time left", time.Until(deadline).String()), slog.Int("utxos", len(b.utxoChannel)), slog.Uint64("mempool txs", mempoolSize))
			case <-submitTicker.C:
				txOut := <-b.utxoChannel

				success := false

				// Try 3 times
				for range 3 {
					hash, satoshis, err = b.processor.SubmitSelfPayingSingleOutputTx(txOut)
					if err != nil {
						if errors.Is(err, context.Canceled) {
							return
						}

						logger.Error("Submitting tx failed", "hash", txOut.Hash.String(), "err", err)
						if strings.Contains(err.Error(), "Transaction outputs already in utxo set") {
							continue mainLoop
						}

						time.Sleep(50 * time.Millisecond)
						continue
					}

					success = true
					break
				}

				if !success {
					continue
				}

				logger.Debug("Submitting tx successful", "hash", hash.String())
				b.utxoChannel <- TxOut{
					Hash:     hash,
					ValueSat: satoshis,
					VOut:     0,
				}

				atomic.AddInt64(&b.totalTxs, 1)
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
