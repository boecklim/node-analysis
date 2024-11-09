package utils

import (
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func PayToAddress(address btcutil.Address, hash *chainhash.Hash, txOut *btcjson.GetTxOutResult, privKey *btcec.PrivateKey) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	prevOut := wire.NewOutPoint(hash, 0)
	input := wire.NewTxIn(prevOut, nil, nil)
	tx.AddTxIn(input)

	pkScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return nil, err
	}

	valueSat := int64(txOut.Value * 1e8)
	tx.AddTxOut(wire.NewTxOut(1000, []byte(pkScript)))
	tx.AddTxOut(wire.NewTxOut(valueSat-1500, []byte(pkScript)))

	lookupKey := func(a btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return privKey, true, nil
	}

	pkScriptOrig, err := hex.DecodeString(txOut.ScriptPubKey.Hex)
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
