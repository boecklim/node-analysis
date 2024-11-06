package utils

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func PayToAddress(addr btcutil.Address, originTx *btcutil.Tx, privKey *btcec.PrivateKey) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	hash := originTx.Hash()

	prevOut := wire.NewOutPoint(hash, 0)
	input := wire.NewTxIn(prevOut, nil, nil)
	tx.AddTxIn(input)

	pkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return nil, err
	}
	tx.AddTxOut(wire.NewTxOut(1000, []byte(pkScript)))

	// toAddr, err := btcutil.DecodeAddress(utxo.Address, &chaincfg.RegressionNetParams)
	// if err != nil {
	// 	return nil, err
	// }

	pkScript2, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return nil, err
	}

	tx.AddTxOut(wire.NewTxOut(int64(originTx.MsgTx().TxOut[0].Value-1500), []byte(pkScript2)))

	lookupKey := func(a btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return privKey, true, nil
	}

	pkScriptOrig := originTx.MsgTx().TxOut[0].PkScript

	sigScript, err := txscript.SignTxOutput(&chaincfg.MainNetParams,
		tx, 0, pkScriptOrig, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil)
	if err != nil {

		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	return tx, nil

}
