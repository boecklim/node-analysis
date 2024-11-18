package broadcaster

import (
	"context"
	"errors"
	"log/slog"
	"node-analysis/utils"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

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
				tx, err := b.createSelfPayingTx(txOut)
				if err != nil {
					b.logger.Error("failed to create self paying tx", slog.String("err", err.Error()))
					b.shutdown <- struct{}{}
					continue
				}

				b.logger.Debug("submitting tx", "hash", tx.TxID())
				hash, err := b.client.SendRawTransaction(tx)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}

					b.logger.Error("submitting tx failed", "hash", tx.TxID())

					txHash := tx.TxHash()

					sat, found := b.satoshiMap.Load(tx.TxID())
					satoshis, isValid := sat.(int64)
					if found && isValid {
						b.utxoChannel <- utils.TxOut{
							Hash:            &txHash,
							ScriptPubKeyHex: b.addressScriptHex,
							ValueSat:        satoshis,
							VOut:            0,
						}
					}

					errCh <- err
					continue
				}

				b.logger.Debug("submitting tx successful", "hash", tx.TxID())
				sat, found := b.satoshiMap.Load(tx.TxID())
				satoshis, isValid := sat.(int64)

				if found && isValid {
					newUtxo := utils.TxOut{
						Hash:            hash,
						ScriptPubKeyHex: b.addressScriptHex,
						ValueSat:        satoshis,
						VOut:            0,
					}
					b.utxoChannel <- newUtxo
				}

				b.satoshiMap.Delete(tx.TxID())

				atomic.AddInt64(&b.totalTxs, 1)

			case responseErr := <-errCh:
				b.logger.Error("failed to submit transactions", slog.String("err", responseErr.Error()))
			}
		}
	}()

	b.wg.Wait()

	return nil
}

func (b *Broadcaster) createSelfPayingTx(txOut utils.TxOut) (*wire.MsgTx, error) {

	b.logger.Debug("creating tx", "prev tx hash", txOut.Hash.String(), "vout", txOut.VOut)

	tx := wire.NewMsgTx(wire.TxVersion)
	amount := txOut.ValueSat

	prevOut := wire.NewOutPoint(txOut.Hash, txOut.VOut)
	input := wire.NewTxIn(prevOut, nil, nil)

	tx.AddTxIn(input)

	amount -= fee

	tx.AddTxOut(wire.NewTxOut(amount, []byte(b.pkScript)))

	lookupKey := func(a btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return b.privKey, true, nil
	}
	sigScript, err := txscript.SignTxOutput(&chaincfg.MainNetParams,
		tx, 0, b.pkScript, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	b.logger.Debug("tx created", "hash", tx.TxID())

	b.satoshiMap.Store(tx.TxID(), tx.TxOut[0].Value)
	return tx, nil
}

func (b *Broadcaster) Shutdown() {
	b.cancelAll()

	b.wg.Wait()
}
