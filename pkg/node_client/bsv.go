package node_client

import (
	"context"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/libsv/go-bk/bec"
	"github.com/libsv/go-bt/v2"
	"github.com/libsv/go-bt/v2/unlocker"

	"github.com/boecklim/node-analysis/pkg/broadcaster"
)

func (p *Processor) splitToAddressBSV(txOut *broadcaster.TxOut, outputs int) (res *splitResult, err error) {
	tx := bt.NewTx()

	err = tx.From(txOut.Hash.String(), txOut.VOut, txOut.ScriptPubKeyHex, uint64(txOut.ValueSat))
	if err != nil {
		return nil, err
	}

	remainingSat := txOut.ValueSat

	satPerOutput := int64(math.Floor(float64(txOut.ValueSat) / float64(outputs+1)))

	for range outputs {
		err = tx.PayToAddress(p.addressString, uint64(satPerOutput))
		if err != nil {
			return nil, err
		}
		remainingSat -= satPerOutput
	}

	err = tx.PayToAddress(p.addressString, uint64(remainingSat-fee))
	if err != nil {
		return nil, err
	}
	privKeyBec, _ := bec.PrivKeyFromBytes(bec.S256(), p.privKey.Serialize())
	err = tx.FillAllInputs(context.Background(), &unlocker.Getter{PrivateKey: privKeyBec})
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %v", err)
	}

	splitOutputs := make([]splitOutput, len(tx.Outputs))
	for i, output := range tx.Outputs {
		splitOutputs[i] = splitOutput{
			pkScript: output.LockingScript.String(),
			satoshis: int64(output.Satoshis),
		}
	}

	hash, err := chainhash.NewHashFromStr(tx.TxID())
	if err != nil {
		return nil, err
	}

	result := &splitResult{
		hash:      hash,
		outputs:   splitOutputs,
		hexString: tx.String(),
	}

	return result, nil
}
