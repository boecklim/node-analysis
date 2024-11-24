package bsv

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"node-analysis/broadcaster"
	"os"
	"time"

	ec "github.com/bitcoin-sv/go-sdk/primitives/ec"
	sdkTx "github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/bitcoin-sv/go-sdk/transaction/template/p2pkh"
	"github.com/bitcoinsv/bsvutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"

	"github.com/ordishs/go-bitcoin"
)

const (
	coinBaseVout = 0
	satPerBtc    = 1e8
	targetUtxos  = 150
)

var _ broadcaster.RPCClient = &Client{}

var (
	ErrOutputSpent = errors.New("output already spent")
)

type Client struct {
	client     *bitcoin.Bitcoind
	logger     *slog.Logger
	privateKey string
	address    string
}

func New(client *bitcoin.Bitcoind) (*Client, error) {
	p := &Client{
		client: client,
		logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	return p, nil
}

func (p *Client) GetCoinbaseTxOutFromBlock(blockHash string) (broadcaster.TxOut, error) {
	lastBlock, err := p.client.GetBlock(blockHash)
	if err != nil {
		return broadcaster.TxOut{}, err
	}

	txHash := lastBlock.Tx[0]

	// USE GETTXOUT https://bitcoin.stackexchange.com/questions/117919/bitcoin-cli-listunspent-returns-empty-list
	txOut, err := p.client.GetTxOut(txHash, coinBaseVout, false)
	if err != nil {
		return broadcaster.TxOut{}, err
	}

	if txOut == nil {
		return broadcaster.TxOut{}, ErrOutputSpent
	}

	hash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return broadcaster.TxOut{}, err
	}

	return broadcaster.TxOut{
		Hash:            hash,
		ValueSat:        int64(txOut.Value * satPerBtc),
		ScriptPubKeyHex: txOut.ScriptPubKey.Hex,
		VOut:            0,
	}, nil
}

func (p *Client) SubmitSelfPayingSingleOutputTx(txOut broadcaster.TxOut) (txHash *chainhash.Hash, satoshis int64, err error) {
	tx := sdkTx.NewTransaction()

	utxo, err := sdkTx.NewUTXO(txOut.Hash.String(), txOut.VOut, txOut.ScriptPubKeyHex, uint64(txOut.ValueSat))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create new utxo: %v", err)
	}

	const fee = 5

	amount := txOut.ValueSat - fee

	err = tx.AddInputsFromUTXOs(utxo)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to add intput to tx: %v", err)
	}

	err = tx.PayToAddress(p.address, uint64(amount))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to add payment output to tx: %v", err)
	}

	err = signAllInputs(tx, p.privateKey)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to sign inputs: %v", err)
	}

	txID, err := p.client.SendRawTransaction(tx.Hex())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send tx: %v", err)
	}

	txHash, err = chainhash.NewHashFromStr(txID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed get tx hash: %v", err)
	}

	return txHash, amount, nil
}

func signAllInputs(tx *sdkTx.Transaction, privateKey string) error {
	// Sign the input
	wif, err := bsvutil.DecodeWIF(privateKey)
	if err != nil {
		return err
	}

	privateKeyDecoded := wif.PrivKey.Serialize()
	pk, _ := ec.PrivateKeyFromBytes(privateKeyDecoded)

	unlockingScriptTemplate, err := p2pkh.Unlock(pk, nil)
	if err != nil {
		return err
	}

	for _, input := range tx.Inputs {
		input.UnlockingScriptTemplate = unlockingScriptTemplate
	}

	err = tx.Sign()
	if err != nil {
		return err
	}

	return nil
}

func (p *Client) PrepareUtxos(utxoChannel chan broadcaster.TxOut) error {
	info, err := p.client.GetInfo()
	if err != nil {
		return fmt.Errorf("failed to get info: %v", err)
	}

	// fund node
	const minNumbeOfBlocks = 101

	if info.Blocks < minNumbeOfBlocks {
		// generate blocks in part to ensure blocktx is able to process all blocks
		const blockBatch = 20 // should be less or equal n*10 where n is number of blocktx instances

		for {
			_, err = p.client.Generate(blockBatch)
			if err != nil {
				return fmt.Errorf("failed to generate block batch: %v", err)
			}

			// give time to send all INV messages
			time.Sleep(5 * time.Second)

			info, err = p.client.GetInfo()
			if err != nil {
				return fmt.Errorf("failed to get info: %v", err)
			}

			missingBlocks := minNumbeOfBlocks - info.Blocks
			if missingBlocks < 0 {
				break
			}
		}
	}

	address, privKey, err := getNewWalletAddress(p.client)
	if err != nil {
		return fmt.Errorf("failed to get new wallet address: %v", err)
	}

	p.address = address
	p.privateKey = privKey

	for len(utxoChannel) < targetUtxos {
		_, err = p.client.SendToAddress(address, 0.001)
		if err != nil {
			return fmt.Errorf("failed to send to address address: %v", err)
		}

		utxos, err := getUtxos(p.client, address)
		if err != nil {
			return fmt.Errorf("failed to get utxos: %v", err)
		}

		tx, err := splitToAddress(privKey, address, utxos, 60)
		if err != nil {
			return fmt.Errorf("failed to split utxo to address: %v", err)
		}

		sentTxHash, err := p.client.SendRawTransaction(tx.Hex())
		if err != nil {
			return fmt.Errorf("failed to send raw transaction: %v", err)
		}

		p.logger.Info("sent raw tx", "hash", sentTxHash, "outputs", len(tx.Outputs))

		for i, output := range tx.Outputs {
			hash, err := chainhash.NewHashFromStr(sentTxHash)
			if err != nil {
				return fmt.Errorf("failed to create tx hash: %v", err)
			}

			txOut := broadcaster.TxOut{
				Hash:            hash,
				ScriptPubKeyHex: hex.EncodeToString(*output.LockingScript),
				ValueSat:        int64(output.Satoshis),
				VOut:            uint32(i),
			}

			utxoChannel <- txOut
		}
	}

	return nil
}

func getNewWalletAddress(bitcoind *bitcoin.Bitcoind) (string, string, error) {
	address, err := bitcoind.GetNewAddress()
	if err != nil {
		return "", "", fmt.Errorf("failed to get new address: %v", err)
	}

	privateKey, err := bitcoind.DumpPrivKey(address)
	if err != nil {
		return "", "", fmt.Errorf("failed to dump private key: %v", err)
	}

	accountName := "test-account"
	err = bitcoind.SetAccount(address, accountName)
	if err != nil {
		return "", "", fmt.Errorf("failed to set account: %v", err)
	}

	return address, privateKey, nil
}

func getUtxos(bitcoind *bitcoin.Bitcoind, address string) ([]UnspentOutput, error) {
	data, err := bitcoind.ListUnspent([]string{address})
	if err != nil {
		return nil, fmt.Errorf("failed to list unspent: %v", err)
	}

	result := make([]UnspentOutput, len(data))

	for index, utxo := range data {
		result[index] = UnspentOutput{
			Txid:          utxo.TXID,
			Vout:          utxo.Vout,
			Address:       utxo.Address,
			ScriptPubKey:  utxo.ScriptPubKey,
			Amount:        utxo.Amount,
			Confirmations: int(utxo.Confirmations),
		}
	}

	return result, nil
}

type UnspentOutput struct {
	Txid          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Address       string  `json:"address"`
	Account       string  `json:"account"`
	ScriptPubKey  string  `json:"scriptPubKey"`
	Amount        float64 `json:"amount"`
	Confirmations int     `json:"confirmations"`
	Spendable     bool    `json:"spendable"`
	Solvable      bool    `json:"solvable"`
	Safe          bool    `json:"safe"`
}

func splitToAddress(privateKey string, address string, utxos []UnspentOutput, outputs int, fee ...uint64) (*sdkTx.Transaction, error) {
	tx := sdkTx.NewTransaction()

	// Add an input using the UTXOs
	for _, utxo := range utxos {
		utxoTxID := utxo.Txid
		utxoVout := utxo.Vout
		utxoSatoshis := uint64(utxo.Amount * 1e8) // Convert BTC to satoshis
		utxoScript := utxo.ScriptPubKey

		u, err := sdkTx.NewUTXO(utxoTxID, utxoVout, utxoScript, utxoSatoshis)
		if err != nil {
			return nil, fmt.Errorf("failed creating UTXO: %v", err)
		}
		err = tx.AddInputsFromUTXOs(u)
		if err != nil {
			return nil, fmt.Errorf("failed adding input: %v", err)
		}
	}
	// Add an output to the address you've previously created
	recipientAddress := address

	var feeValue uint64
	if len(fee) > 0 {
		feeValue = fee[0]
	} else {
		feeValue = 20 // Set your default fee value here
	}

	totalSat, err := tx.TotalInputSatoshis()
	if err != nil {
		return nil, fmt.Errorf("failed go get total input satoshis: %v", err)
	}
	remainingSat := totalSat
	satPerOutput := uint64(math.Floor(float64(totalSat) / float64(outputs+1)))
	for range outputs {
		err = tx.PayToAddress(recipientAddress, satPerOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to pay to address: %v", err)
		}

		remainingSat -= satPerOutput
	}

	err = tx.PayToAddress(recipientAddress, satPerOutput-feeValue)
	if err != nil {
		return nil, fmt.Errorf("failed to add payment output: %v", err)
	}

	// Sign the input
	wif, err := bsvutil.DecodeWIF(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode WIF: %v", err)
	}

	// Extract raw private key bytes directly from the WIF structure
	privateKeyDecoded := wif.PrivKey.Serialize()
	pk, _ := ec.PrivateKeyFromBytes(privateKeyDecoded)

	unlockingScriptTemplate, err := p2pkh.Unlock(pk, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to unlock script: %v", err)
	}

	for _, input := range tx.Inputs {
		input.UnlockingScriptTemplate = unlockingScriptTemplate
	}

	err = tx.Sign()
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx: %v", err)
	}

	return tx, nil
}
