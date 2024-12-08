package bsv

import (
	"encoding/hex"
	"errors"
	"fmt"
	keyset "github.com/boecklim/node-analysis/key_set"
	"github.com/boecklim/node-analysis/processor"
	"log/slog"
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
	outputsPerTx = 50
)

var _ processor.RPCClient = &Client{}

var (
	ErrOutputSpent = errors.New("output already spent")
)

type Client struct {
	client     *bitcoin.Bitcoind
	logger     *slog.Logger
	privateKey *ec.PrivateKey
	address    string
}

func New(client *bitcoin.Bitcoind, logger *slog.Logger) (*Client, error) {
	p := &Client{
		client: client,
		logger: logger,
	}

	err := p.setAddress()
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Client) setAddress() error {
	ks, err := keyset.NewFromExtendedKeyStr("xprv9s21ZrQH143K2yZtKVRuSXDr1hXNWPciLsRi7SFB5JzY9Z4tAMWUpWdRWhpcqB5ESyGagKQbcejAcj5eQHD8Dej1uYrHYbmma5VEtnTAtg4", "0/0")
	if err != nil {
		return err
	}
	p.address = ks.Address(false)

	p.privateKey = ks.PrivateKey

	return nil
}
func (p *Client) GetBlockSize(blockHash *chainhash.Hash) (sizeBytes uint64, nrTxs uint64, err error) {

	blockMsg, err := p.client.GetBlock(blockHash.String())
	if err != nil {
		return 0, 0, err
	}

	return blockMsg.Size, blockMsg.NumTx, nil
}
func (p *Client) GetCoinbaseTxOutFromBlock(blockHash string) (processor.TxOut, error) {
	lastBlock, err := p.client.GetBlock(blockHash)
	if err != nil {
		return processor.TxOut{}, err
	}

	txHash := lastBlock.Tx[0]

	// USE GETTXOUT https://bitcoin.stackexchange.com/questions/117919/bitcoin-cli-listunspent-returns-empty-list
	txOut, err := p.client.GetTxOut(txHash, coinBaseVout, false)
	if err != nil {
		return processor.TxOut{}, err
	}

	if txOut == nil {
		return processor.TxOut{}, ErrOutputSpent
	}

	hash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return processor.TxOut{}, err
	}

	return processor.TxOut{
		Hash:            hash,
		ValueSat:        int64(txOut.Value * satPerBtc),
		ScriptPubKeyHex: txOut.ScriptPubKey.Hex,
		VOut:            0,
	}, nil
}

func (p *Client) SubmitSelfPayingSingleOutputTx(txOut processor.TxOut) (txHash *chainhash.Hash, satoshis int64, err error) {
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

	err = signAllInputs(tx, p.privateKey.Wif())
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

func (p *Client) PrepareUtxos(utxoChannel chan processor.TxOut, targetUtxos int) (err error) {
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

	fundingTxID, err := p.client.SendToAddress(p.address, 20)
	if err != nil {
		return fmt.Errorf("failed to send to address address: %v", err)
	}
	rawTx, err := p.client.GetRawTransaction(fundingTxID)
	if err != nil {
		return fmt.Errorf("failed to get raw tx: %v", err)
	}

	utxo, err := sdkTx.NewUTXO(rawTx.TxID, 1, rawTx.Vout[1].ScriptPubKey.Hex, uint64(rawTx.Vout[1].Value*satPerBtc))
	if err != nil {
		return fmt.Errorf("failed creating UTXO: %v", err)
	}

	for len(utxoChannel) < targetUtxos {
		var tx *sdkTx.Transaction
		tx, err = p.splitToAddress(utxo, outputsPerTx)
		if err != nil {
			return fmt.Errorf("failed to split utxo to address: %v", err)
		}

		fmt.Println(tx.Hex())

		var sentTxHash string
		sentTxHash, err = p.client.SendRawTransaction(tx.Hex())
		if err != nil {
			return fmt.Errorf("failed to send raw transaction: %v", err)
		}

		p.logger.Info("sent raw tx", "hash", sentTxHash, "outputs", len(tx.Outputs))

		for i, output := range tx.Outputs {
			if i == len(tx.Outputs)-1 {
				utxo, err = sdkTx.NewUTXO(tx.TxID().String(), uint32(i), output.LockingScriptHex(), output.Satoshis)
				if err != nil {
					return fmt.Errorf("failed to create UTXO: %v", err)
				}
				break
			}

			hash, err := chainhash.NewHashFromStr(sentTxHash)
			if err != nil {
				return fmt.Errorf("failed to create tx hash: %v", err)
			}

			txOut := processor.TxOut{
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
			Txid:         utxo.TXID,
			Vout:         utxo.Vout,
			ScriptPubKey: utxo.ScriptPubKey,
			Amount:       utxo.Amount,
		}
	}

	return result, nil
}

type UnspentOutput struct {
	Txid         string  `json:"txid"`
	Vout         uint32  `json:"vout"`
	ScriptPubKey string  `json:"scriptPubKey"`
	Amount       float64 `json:"amount"`
}

func (p *Client) splitToAddress(utxo *sdkTx.UTXO, outputs int) (*sdkTx.Transaction, error) {
	tx := sdkTx.NewTransaction()

	err := tx.AddInputsFromUTXOs(utxo)
	if err != nil {
		return nil, fmt.Errorf("failed adding input: %v", err)
	}
	// Add an output to the address you've previously created

	const feeValue = 20 // Set your default fee value here
	const satPerOutput = 1000

	totalSat, err := tx.TotalInputSatoshis()
	if err != nil {
		return nil, fmt.Errorf("failed go get total input satoshis: %v", err)
	}
	remainingSat := totalSat
	for range outputs {
		err = tx.PayToAddress(p.address, satPerOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to pay to address: %v", err)
		}

		remainingSat -= satPerOutput
	}

	err = tx.PayToAddress(p.address, remainingSat-feeValue)
	if err != nil {
		return nil, fmt.Errorf("failed to add payment output: %v", err)
	}

	// Sign the input
	wif, err := bsvutil.DecodeWIF(p.privateKey.Wif())
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

func (p *Client) GenerateBlock() (blockID string, err error) {
	blockHash, err := p.client.Generate(1)
	if err != nil {
		return blockID, err
	}

	return blockHash[0], nil
}
