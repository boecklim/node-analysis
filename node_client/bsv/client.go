package bsv

import (
	"encoding/hex"
	"errors"
	"fmt"
	ec "github.com/bitcoin-sv/go-sdk/primitives/ec"
	sdkTx "github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/bitcoin-sv/go-sdk/transaction/template/p2pkh"
	"github.com/bitcoinsv/bsvutil"
	keyset "github.com/boecklim/node-analysis/key_set"
	"github.com/boecklim/node-analysis/processor"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"log/slog"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/ordishs/go-bitcoin"
)

const (
	coinBaseVout               = 0
	satPerBtc                  = 1e8
	outputsPerTx               = 20
	coinbaseSpendableAfterConf = 200
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

func (p *Client) GetMempoolSize() (nrTxs uint64, err error) {
	info, err := p.client.GetMempoolInfo()
	if err != nil {
		return 0, err
	}

	return uint64(info.Size), nil
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

func (p *Client) getCoinbaseTxOut() (*processor.TxOut, error) {
	var txOut *bitcoin.TXOut
	var txHash string
	var err error

	// Find a coinbase tx out which has not been spent yet
	for {
		height := rand.Intn(200)
		blockHash, err := p.client.GetBlockHash(height)
		if err != nil {
			return nil, fmt.Errorf("failed go get block hash at height %d: %v", height, err)
		}

		coinbase, err := p.client.GetBlockHeaderAndCoinbase(blockHash)
		if err != nil {
			return nil, fmt.Errorf("failed go get block header for block hash %s: %v", blockHash, err)
		}

		txHash = coinbase.Tx[0].TxID

		txOut, err = p.client.GetTxOut(txHash, coinBaseVout, true)
		if err != nil {
			return nil, err
		}

		if txOut != nil {
			break
		}
	}

	hash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get hash: %v", err)
	}

	return &processor.TxOut{
		Hash:            hash,
		ValueSat:        int64(txOut.Value * satPerBtc),
		ScriptPubKeyHex: txOut.ScriptPubKey.Hex,
		VOut:            0,
	}, nil
}

func (p *Client) getBlockHeight() (int64, error) {
	info, err := p.client.GetMiningInfo()
	if err != nil {
		return 0, fmt.Errorf("failed to get info: %v", err)
	}

	return int64(info.Blocks), nil
}

func (p *Client) PrepareUtxos(utxoChannel chan processor.TxOut, targetUtxos int) (err error) {
	time.Sleep(time.Duration(rand.Intn(30)) * time.Second) // Wait a random number of seconds to avoid fork at start

	blockHeight, err := p.getBlockHeight()
	if err != nil {
		return fmt.Errorf("failed to get info: %v", err)
	}

	signalFinish := make(chan struct{})
	loggingStopped := make(chan struct{})
	showTicker := time.NewTicker(2 * time.Second)
	go func() {
		defer close(loggingStopped)

		for {
			select {
			case <-showTicker.C:
				p.logger.Info("Creating utxos", slog.Int("count", len(utxoChannel)), slog.Int("target", targetUtxos))
			case <-signalFinish:
				return
			}
		}
	}()

	if blockHeight <= 500 {
		blocksToGenerate := 500 - blockHeight
		p.logger.Info("Generating blocks", "number", blocksToGenerate)
		_, err = p.client.GenerateToAddress(float64(blocksToGenerate), p.address)
		if err != nil {
			return fmt.Errorf("failed to gnereate to address: %v", err)
		}
	}
outerLoop:
	for len(utxoChannel) < targetUtxos {
		var rootTxOut *processor.TxOut
		rootTxOut, err = p.getCoinbaseTxOut()
		if err != nil {
			return fmt.Errorf("failed to get coinbaise tx out: %v", err)
		}

		p.logger.Debug("Splittable output", "hash", rootTxOut.Hash.String(), "value", rootTxOut.ValueSat)

		rootTx, err := p.splitToAddress(rootTxOut, outputsPerTx)
		if err != nil {
			return fmt.Errorf("failed split to address: %v", err)
		}

		var sentTxHash string
		sentTxHash, err = p.client.SendRawTransaction(rootTx.Hex())
		if err != nil {
			if strings.Contains(err.Error(), "mandatory-script-verify-flag-failed") {
				p.logger.Error("Failed to send root tx", "err", err)

				continue
			}

			return fmt.Errorf("failed to send root tx: %v", err)
		}

		p.logger.Info("Sent root tx", "hash", sentTxHash, "outputs", len(rootTx.Outputs))

		hash, err := chainhash.NewHash(rootTx.TxID()[:])
		if err != nil {
			return fmt.Errorf("failed get hash: %v", err)
		}

		var splitTxOut *processor.TxOut

		for rootIndex, rootOutput := range rootTx.Outputs {
			splitTxOut = &processor.TxOut{
				Hash:            hash,
				ValueSat:        int64(rootOutput.Satoshis),
				ScriptPubKeyHex: hex.EncodeToString(rootOutput.LockingScript.Bytes()),
				VOut:            uint32(rootIndex),
			}

			splitTx1, err := p.splitToAddress(splitTxOut, outputsPerTx)
			if err != nil {
				continue
			}

			sentTxHash1, err := p.client.SendRawTransaction(splitTx1.Hex())
			if err != nil {
				return fmt.Errorf("failed to send splitTx1 tx: %v", err)
			}

			p.logger.Debug("Sent split tx 2", "hash", splitTx1.TxID(), "outputs", len(splitTx1.Outputs))

			newHash1, err := chainhash.NewHashFromStr(sentTxHash1)
			if err != nil {
				return fmt.Errorf("failed get hash: %v", err)
			}

			var splitTxOut2 *processor.TxOut
			for index1, output1 := range splitTx1.Outputs {
				if len(utxoChannel) >= targetUtxos {
					break outerLoop
				}
				splitTxOut2 = &processor.TxOut{
					Hash:            newHash1,
					ValueSat:        int64(output1.Satoshis),
					ScriptPubKeyHex: hex.EncodeToString(output1.LockingScript.Bytes()),
					VOut:            uint32(index1),
				}

				splitTx2, err := p.splitToAddress(splitTxOut2, outputsPerTx)
				if err != nil {
					continue
				}

				sentTxHash2, err := p.client.SendRawTransaction(splitTx2.Hex())
				if err != nil {
					return fmt.Errorf("failed to send splitTx1 tx: %v", err)
				}

				p.logger.Debug("Sent split tx 2", "hash", splitTx1.TxID(), "outputs", len(splitTx1.Outputs))
				for index2, output2 := range splitTx2.Outputs {
					if len(utxoChannel) >= targetUtxos {
						break outerLoop
					}

					newHash2, err := chainhash.NewHashFromStr(sentTxHash2)
					if err != nil {
						return fmt.Errorf("failed get hash: %v", err)
					}

					utxoChannel <- processor.TxOut{
						Hash:            newHash2,
						ScriptPubKeyHex: output2.LockingScriptHex(),
						ValueSat:        int64(output2.Satoshis),
						VOut:            uint32(index2),
					}
				}
			}
		}
	}

	bhs, err := p.client.GenerateToAddress(float64(1), p.address)
	if err != nil {
		return fmt.Errorf("failed to gnereate to address: %v", err)
	}

	p.logger.Info("Generated new block", "hash", bhs[0])

	close(signalFinish)
	<-loggingStopped
	p.logger.Info("Created utxos", slog.Int("count", len(utxoChannel)), slog.Int("target", targetUtxos))

	return nil
}

func (p *Client) splitToAddress(txOut *processor.TxOut, outputs int) (*sdkTx.Transaction, error) {
	utxo, err := sdkTx.NewUTXO(txOut.Hash.String(), txOut.VOut, txOut.ScriptPubKeyHex, uint64(txOut.ValueSat))
	if err != nil {
		return nil, fmt.Errorf("failed to create utxo: %v", err)
	}

	tx := sdkTx.NewTransaction()

	err = tx.AddInputsFromUTXOs(utxo)
	if err != nil {
		return nil, fmt.Errorf("failed adding input: %v", err)
	}
	// Add an output to the address you've previously created

	const feeValue = 20 // Set your default fee value here
	satPerOutput := uint64(math.Floor(float64(txOut.ValueSat) / float64(outputs+1)))

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
