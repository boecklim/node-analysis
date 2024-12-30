package node_client

import (
	"context"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/libsv/go-bk/bec"
	chaincfgSV "github.com/libsv/go-bk/chaincfg"
	"github.com/libsv/go-bk/wif"
	"github.com/libsv/go-bt/v2"
	"github.com/libsv/go-bt/v2/bscript"
	"github.com/libsv/go-bt/v2/unlocker"

	"github.com/boecklim/node-analysis/pkg/processor"
)

func (p *Processor) createSelfPayingTxBSV(txOut processor.TxOut) (*selfPayingResult, error) {
	if txOut.Hash == nil {
		return nil, fmt.Errorf("hash is missing")
	}

	p.logger.Debug("creating tx", "prev tx hash", txOut.Hash.String(), "vout", txOut.VOut)

	tx := bt.NewTx()

	err := tx.From(txOut.Hash.String(), txOut.VOut, txOut.ScriptPubKeyHex, uint64(txOut.ValueSat))
	if err != nil {
		return nil, err
	}

	amount := txOut.ValueSat
	amount -= fee

	err = tx.PayToAddress(p.addressString, uint64(amount))
	if err != nil {
		return nil, err
	}

	err = tx.FillAllInputs(context.Background(), &unlocker.Getter{PrivateKey: p.privKeyBSV})
	if err != nil {
		return nil, err
	}

	p.logger.Debug("tx created", "hash", tx.TxID())

	txHash, err := chainhash.NewHashFromStr(tx.TxID())

	if err != nil {
		return nil, err
	}

	return &selfPayingResult{
		hash:      txHash,
		satoshis:  int64(tx.Outputs[0].Satoshis),
		hexString: tx.String(),
	}, nil
}

func (p *Processor) splitToAddressBSV(txOut *processor.TxOut, outputs int) (res splitResult, err error) {
	tx := bt.NewTx()

	err = tx.From(txOut.Hash.String(), txOut.VOut, txOut.ScriptPubKeyHex, uint64(txOut.ValueSat))
	if err != nil {
		return splitResult{}, err
	}

	remainingSat := txOut.ValueSat

	satPerOutput := int64(math.Floor(float64(txOut.ValueSat) / float64(outputs+1)))

	for range outputs {
		err = tx.PayToAddress(p.addressString, uint64(satPerOutput))
		if err != nil {
			return splitResult{}, err
		}
		remainingSat -= satPerOutput
	}

	err = tx.PayToAddress(p.addressString, uint64(remainingSat-fee))
	if err != nil {
		return splitResult{}, err
	}

	err = tx.FillAllInputs(context.Background(), &unlocker.Getter{PrivateKey: p.privKeyBSV})
	if err != nil {
		return splitResult{}, err
	}

	splitOutputs := make([]splitOutput, len(tx.Outputs))
	for i, output := range tx.Outputs {
		splitOutputs[i] = splitOutput{
			PkScript: output.LockingScript.String(),
			Value:    int64(output.Satoshis),
		}
	}

	hash, err := chainhash.NewHashFromStr(tx.TxID())
	if err != nil {
		return splitResult{}, err
	}

	result := splitResult{
		hash:      hash,
		outputs:   splitOutputs,
		hexString: tx.String(),
	}

	return result, nil
}

func (p *Processor) setAddressBSV() error {
	privKey, err := bec.NewPrivateKey(bec.S256())
	if err != nil {
		return err
	}

	newWif, err := wif.NewWIF(privKey, &chaincfgSV.TestNet, false)
	if err != nil {
		return err
	}

	address, err := bscript.NewAddressFromPublicKey(newWif.PrivKey.PubKey(), false)
	if err != nil {
		return err
	}

	p.addressBSV = *address
	p.privKeyBSV = newWif.PrivKey
	p.addressString = address.AddressString

	return nil
}
