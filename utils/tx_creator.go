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

func SplitToAddress(address btcutil.Address, prevOutputHash *chainhash.Hash, inputSat int64, scriptPubKeyHex string, outputs int, privKey *btcec.PrivateKey) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	prevOut := wire.NewOutPoint(prevOutputHash, 0)
	input := wire.NewTxIn(prevOut, nil, nil)
	tx.AddTxIn(input)

	pkScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return nil, err
	}

	remainingSat := inputSat

	satPerOutput := int64(math.Floor(float64(inputSat) / float64(outputs+1)))

	for range outputs {
		tx.AddTxOut(wire.NewTxOut(satPerOutput, []byte(pkScript)))
		remainingSat -= satPerOutput
	}

	tx.AddTxOut(wire.NewTxOut(remainingSat-3000, []byte(pkScript)))

	lookupKey := func(a btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return privKey, true, nil
	}

	pkScriptOrig, err := hex.DecodeString(scriptPubKeyHex)
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
