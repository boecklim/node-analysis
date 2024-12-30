package node_client

import (
	"encoding/hex"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"

	"github.com/boecklim/node-analysis/pkg/processor"
)

func (p *Processor) setAddressBTC() error {
	var err error
	var privKey *btcec.PrivateKey

	privKey, err = btcec.NewPrivateKey()
	if err != nil {
		return fmt.Errorf("failed to create private key: %w", err)
	}

	address, err := btcutil.NewAddressPubKey(privKey.PubKey().SerializeCompressed(),
		&chaincfg.RegressionNetParams)
	if err != nil {
		return err
	}

	p.address = address
	p.privKey = privKey
	p.addressString = address.EncodeAddress()

	p.logger.Info("New address", "address", p.addressString)

	pkScript, err := txscript.PayToAddrScript(p.address)
	if err != nil {
		return err
	}

	p.pkScript = pkScript
	return nil
}

func (p *Processor) createSelfPayingTxBTC(txOut processor.TxOut) (*selfPayingResult, error) {
	if txOut.Hash == nil {
		return nil, fmt.Errorf("hash is missing")
	}

	p.logger.Debug("creating tx", "prev tx hash", txOut.Hash.String(), "vout", txOut.VOut)

	tx := wire.NewMsgTx(wire.TxVersion)
	amount := txOut.ValueSat

	prevOut := wire.NewOutPoint(txOut.Hash, txOut.VOut)
	input := wire.NewTxIn(prevOut, nil, nil)

	tx.AddTxIn(input)

	amount -= fee

	tx.AddTxOut(wire.NewTxOut(amount, p.pkScript))

	lookupKey := func(_ btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return p.privKey, true, nil
	}
	sigScript, err := txscript.SignTxOutput(&chaincfg.RegressionNetParams,
		tx, 0, p.pkScript, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	p.logger.Debug("tx created", "hash", tx.TxID())

	hexString, err := getHexString(tx)
	if err != nil {
		return nil, err
	}

	txHash := tx.TxHash()

	return &selfPayingResult{
		hash:      &txHash,
		satoshis:  tx.TxOut[0].Value,
		hexString: hexString,
	}, nil
}

func (p *Processor) splitToAddressBTC(txOut *processor.TxOut, outputs int) (res splitResult, err error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	prevOut := wire.NewOutPoint(txOut.Hash, txOut.VOut)
	input := wire.NewTxIn(prevOut, nil, nil)
	tx.AddTxIn(input)

	pkScript, err := txscript.PayToAddrScript(p.address)
	if err != nil {
		return splitResult{}, err
	}

	remainingSat := txOut.ValueSat

	satPerOutput := int64(math.Floor(float64(txOut.ValueSat) / float64(outputs+1)))

	for range outputs {
		tx.AddTxOut(wire.NewTxOut(satPerOutput, pkScript))
		remainingSat -= satPerOutput
	}

	tx.AddTxOut(wire.NewTxOut(remainingSat-fee, pkScript))

	lookupKey := func(_ btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return p.privKey, true, nil
	}

	pkScriptOrig, err := hex.DecodeString(txOut.ScriptPubKeyHex)
	if err != nil {
		return splitResult{}, err
	}

	sigScript, err := txscript.SignTxOutput(&chaincfg.RegressionNetParams,
		tx, 0, pkScriptOrig, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil)
	if err != nil {
		return splitResult{}, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	hexString, err := getHexString(tx)
	if err != nil {
		return splitResult{}, err
	}

	splitOutputs := make([]splitOutput, len(tx.TxOut))
	for i, output := range tx.TxOut {
		splitOutputs[i] = splitOutput{
			PkScript: hex.EncodeToString(output.PkScript),
			Value:    output.Value,
		}
	}

	hash, err := chainhash.NewHashFromStr(tx.TxID())
	if err != nil {
		return splitResult{}, err
	}

	result := splitResult{
		hash:      hash,
		outputs:   splitOutputs,
		hexString: hexString,
	}

	return result, nil
}
