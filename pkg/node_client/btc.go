package node_client

import (
	"encoding/hex"
	"math"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"

	"github.com/boecklim/node-analysis/pkg/processor"
)

//
//func (p *Processor) createSelfPayingTxBTC(txOut *processor.TxOut) (*selfPayingResult, error) {
//	if txOut.Hash == nil {
//		return nil, fmt.Errorf("hash is missing")
//	}
//
//	p.logger.Debug("creating tx", "prev tx hash", txOut.Hash.String(), "vout", txOut.VOut)
//
//	tx := wire.NewMsgTx(wire.TxVersion)
//	amount := txOut.ValueSat
//
//	prevOut := wire.NewOutPoint(txOut.Hash, txOut.VOut)
//	input := wire.NewTxIn(prevOut, nil, nil)
//
//	tx.AddTxIn(input)
//
//	amount -= fee
//
//	address, err := btcutil.NewAddressPubKey(p.privKey.PubKey().SerializeCompressed(),
//		&chaincfg.RegressionNetParams)
//	if err != nil {
//		return nil, err
//	}
//
//	pkScript, err := txscript.PayToAddrScript(address)
//	if err != nil {
//		return nil, err
//	}
//
//	tx.AddTxOut(wire.NewTxOut(amount, pkScript))
//
//	lookupKey := func(_ btcutil.Address) (*btcec.PrivateKey, bool, error) {
//		return p.privKey, true, nil
//	}
//	sigScript, err := txscript.SignTxOutput(&chaincfg.RegressionNetParams,
//		tx, 0, pkScript, txscript.SigHashAll,
//		txscript.KeyClosure(lookupKey), nil, nil)
//	if err != nil {
//		return nil, err
//	}
//	tx.TxIn[0].SignatureScript = sigScript
//
//	p.logger.Debug("tx created", "hash", tx.TxID())
//
//	hexString, err := getHexString(tx)
//	if err != nil {
//		return nil, err
//	}
//
//	txHash := tx.TxHash()
//
//	return &selfPayingResult{
//		hash:      &txHash,
//		satoshis:  tx.TxOut[0].Value,
//		hexString: hexString,
//	}, nil
//}

func (p *Processor) splitToAddressBTC(txOut *processor.TxOut, outputs int) (res *splitResult, err error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	prevOut := wire.NewOutPoint(txOut.Hash, txOut.VOut)
	input := wire.NewTxIn(prevOut, nil, nil)
	tx.AddTxIn(input)

	address, err := btcutil.NewAddressPubKey(p.privKey.PubKey().SerializeCompressed(),
		&chaincfg.RegressionNetParams)
	if err != nil {
		return nil, err
	}

	pkScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return nil, err
	}

	//pkScript, err := txscript.PayToAddrScript(p.address)
	//if err != nil {
	//	return nil, err
	//}

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
		return nil, err
	}

	sigScript, err := txscript.SignTxOutput(&chaincfg.RegressionNetParams,
		tx, 0, pkScriptOrig, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	hexString, err := getHexString(tx)
	if err != nil {
		return nil, err
	}

	splitOutputs := make([]splitOutput, len(tx.TxOut))
	for i, output := range tx.TxOut {
		splitOutputs[i] = splitOutput{
			pkScript: hex.EncodeToString(output.PkScript),
			satoshis: output.Value,
		}
	}

	hash, err := chainhash.NewHashFromStr(tx.TxID())
	if err != nil {
		return nil, err
	}

	result := &splitResult{
		hash:      hash,
		outputs:   splitOutputs,
		hexString: hexString,
	}

	return result, nil
}
