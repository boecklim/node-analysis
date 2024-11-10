package utils

import (
	"encoding/hex"
	"math"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func SplitToAddress(address btcutil.Address, txOut TxOut, outputs int, privKey *btcec.PrivateKey, fee int64) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	prevOut := wire.NewOutPoint(txOut.Hash, 0)
	input := wire.NewTxIn(prevOut, nil, nil)
	tx.AddTxIn(input)

	pkScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return nil, err
	}

	remainingSat := txOut.ValueSat

	satPerOutput := int64(math.Floor(float64(txOut.ValueSat) / float64(outputs+1)))

	for range outputs {
		tx.AddTxOut(wire.NewTxOut(satPerOutput, []byte(pkScript)))
		remainingSat -= satPerOutput
	}

	tx.AddTxOut(wire.NewTxOut(remainingSat-fee, []byte(pkScript)))

	lookupKey := func(a btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return privKey, true, nil
	}

	pkScriptOrig, err := hex.DecodeString(txOut.ScriptPubKeyHex)
	if err != nil {
		return nil, err
	}

	sigScript, err := txscript.SignTxOutput(&chaincfg.MainNetParams,
		tx, 0, pkScriptOrig, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	return tx, nil

}

type TxOut struct {
	Hash            *chainhash.Hash
	ScriptPubKeyHex string
	ValueSat        int64
	VOut            uint32
}
