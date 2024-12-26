package btc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/boecklim/node-analysis/processor"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	coinBaseVout               = 0
	satPerBtc                  = 1e8
	coinbaseSpendableAfterConf = 100
	outputsPerTx               = 20
	fee                        = 3000
)

var _ processor.RPCClient = &Client{}

var (
	ErrOutputSpent = errors.New("output already spent")
)

// GetMiningInfoResult models the data from the getmininginfo command.
type GetMiningInfoResult struct {
	Blocks             int64   `json:"blocks"`
	CurrentBlockSize   uint64  `json:"currentblocksize"`
	CurrentBlockWeight uint64  `json:"currentblockweight"`
	CurrentBlockTx     uint64  `json:"currentblocktx"`
	Difficulty         float64 `json:"difficulty"`
	Errors             string  `json:"errors"`
	Generate           bool    `json:"generate"`
	GenProcLimit       int32   `json:"genproclimit"`
	HashesPerSec       float64 `json:"hashespersec"`
	NetworkHashPS      float64 `json:"networkhashps"`
	PooledTx           uint64  `json:"pooledtx"`
	TestNet            bool    `json:"testnet"`
}
type GetBlockVerboseResult struct {
	Hash          string   `json:"hash"`
	Confirmations int64    `json:"confirmations"`
	StrippedSize  int32    `json:"strippedsize"`
	Size          int32    `json:"size"`
	Weight        int32    `json:"weight"`
	Height        int64    `json:"height"`
	Version       int32    `json:"version"`
	VersionHex    string   `json:"versionHex"`
	MerkleRoot    string   `json:"merkleroot"`
	Tx            []string `json:"tx,omitempty"`
	Time          int64    `json:"time"`
	Nonce         uint32   `json:"nonce"`
	Bits          string   `json:"bits"`
	Difficulty    float64  `json:"difficulty"`
	PreviousHash  string   `json:"previousblockhash"`
	NextHash      string   `json:"nextblockhash,omitempty"`
}
type GetNetworkInfoResult struct {
	Version         int32                  `json:"version"`
	SubVersion      string                 `json:"subversion"`
	ProtocolVersion int32                  `json:"protocolversion"`
	LocalServices   string                 `json:"localservices"`
	LocalRelay      bool                   `json:"localrelay"`
	TimeOffset      int64                  `json:"timeoffset"`
	Connections     int32                  `json:"connections"`
	ConnectionsIn   int32                  `json:"connections_in"`
	ConnectionsOut  int32                  `json:"connections_out"`
	NetworkActive   bool                   `json:"networkactive"`
	Networks        []NetworksResult       `json:"networks"`
	RelayFee        float64                `json:"relayfee"`
	IncrementalFee  float64                `json:"incrementalfee"`
	LocalAddresses  []LocalAddressesResult `json:"localaddresses"`
	Warnings        StringOrArray          `json:"warnings"`
}

type StringOrArray []string

type LocalAddressesResult struct {
	Address string `json:"address"`
	Port    uint16 `json:"port"`
	Score   int32  `json:"score"`
}

type NetworksResult struct {
	Name                      string `json:"name"`
	Limited                   bool   `json:"limited"`
	Reachable                 bool   `json:"reachable"`
	Proxy                     string `json:"proxy"`
	ProxyRandomizeCredentials bool   `json:"proxy_randomize_credentials"`
}

// GetTxOutResult models the data from the gettxout command.
type GetTxOutResult struct {
	BestBlock     string             `json:"bestblock"`
	Confirmations int64              `json:"confirmations"`
	Value         float64            `json:"value"`
	ScriptPubKey  ScriptPubKeyResult `json:"scriptPubKey"`
	Coinbase      bool               `json:"coinbase"`
}
type ScriptPubKeyResult struct {
	Asm       string   `json:"asm"`
	Hex       string   `json:"hex,omitempty"`
	ReqSigs   int32    `json:"reqSigs,omitempty"` // Deprecated: removed in Bitcoin Core
	Type      string   `json:"type"`
	Address   string   `json:"address,omitempty"`
	Addresses []string `json:"addresses,omitempty"` // Deprecated: removed in Bitcoin Core
}

type RPCClient interface {
	GenerateToAddress(nBlocks int64, address string) ([]string, error)
	GetMiningInfo() (*GetMiningInfoResult, error)
	GetBlock(blockHash string) (*GetBlockVerboseResult, error)
	GetBlockHash(blockHeight int64) (*string, error)
	GetTxOut(txHash string, index uint32, mempool bool) (*GetTxOutResult, error)
	SendRawTransaction(hexString string) (*string, error)
	GetRawMempool() ([]string, error)
}

type Client struct {
	client RPCClient
	logger *slog.Logger

	pkScript []byte
	address  btcutil.Address
	privKey  *btcec.PrivateKey
}

func New(client RPCClient, logger *slog.Logger) (*Client, error) {
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

	p.logger.Info("New address", "address", address.EncodeAddress())

	pkScript, err := txscript.PayToAddrScript(p.address)
	if err != nil {
		return err
	}

	p.pkScript = pkScript
	return nil
}

func (p *Client) getCoinbaseTxOut() (*processor.TxOut, error) {
	var txOut *GetTxOutResult
	var txHash string

	counter := 0

	// Find a coinbase tx out which has not been spent yet
	for {
		bhs, err := p.client.GenerateToAddress(1, p.address.EncodeAddress())
		if err != nil {
			return nil, fmt.Errorf("failed to gnereate to address: %v", err)
		}
		p.logger.Info("Generated new block", "hash", bhs[0])

		if counter > 10 {
			return nil, errors.New("failed to find coinbase tx out")
		}

		blockHeight, err := p.getBlockHeight()
		if err != nil {
			return nil, fmt.Errorf("failed to get info: %v", err)
		}

		bh := blockHeight - coinbaseSpendableAfterConf
		blockHash, err := p.client.GetBlockHash(bh)
		if err != nil {
			return nil, fmt.Errorf("failed go get block hash at height %d: %v", bh, err)
		}

		block, err := p.client.GetBlock(*blockHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get block for hash %s: %v", *blockHash, err)
		}

		txHash = block.Tx[0]

		txOut, err = p.client.GetTxOut(txHash, coinBaseVout, false)
		if err != nil {
			return nil, err
		}

		if txOut != nil {
			break
		}

		counter++
	}

	hash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return nil, err
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

	return info.Blocks, nil
}

func getHexString(tx *wire.MsgTx) (string, error) {
	buf := bytes.Buffer{}
	err := tx.Serialize(&buf)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(buf.Bytes()), nil
}

func (p *Client) SubmitSelfPayingSingleOutputTx(txOut processor.TxOut) (txHash *chainhash.Hash, satoshis int64, err error) {
	tx, err := p.createSelfPayingTx(txOut)
	if err != nil {
		return nil, 0, err
	}

	hexString, err := getHexString(tx)
	if err != nil {
		return nil, 0, err
	}

	hash, err := p.client.SendRawTransaction(hexString)
	if err != nil {
		return nil, 0, err
	}

	txHash, err = chainhash.NewHashFromStr(*hash)
	if err != nil {
		return nil, 0, err
	}
	return txHash, tx.TxOut[0].Value, nil
}

func (p *Client) createSelfPayingTx(txOut processor.TxOut) (*wire.MsgTx, error) {
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

	return tx, nil
}

func (p *Client) GetBlockSize(blockHash *chainhash.Hash) (sizeBytes uint64, nrTxs uint64, err error) {
	blockMsg, err := p.client.GetBlock(blockHash.String())
	if err != nil {
		return 0, 0, err
	}

	return uint64(blockMsg.Size), uint64(len(blockMsg.Tx)), nil
}

func (p *Client) GetMempoolSize() (nrTxs uint64, err error) {
	rawMempool, err := p.client.GetRawMempool()
	if err != nil {
		return 0, err
	}

	return uint64(len(rawMempool)), nil
}

func (p *Client) PrepareUtxos(utxoChannel chan processor.TxOut, targetUtxos int) (err error) {
	blocks, err := p.getBlockHeight()
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

	if blocks <= coinbaseSpendableAfterConf {
		blocksToGenerate := coinbaseSpendableAfterConf + 1 - blocks
		p.logger.Info("Generating blocks", "number", blocksToGenerate)

		_, err = p.client.GenerateToAddress(blocksToGenerate, p.address.EncodeAddress())
		if err != nil {
			return fmt.Errorf("failed to gnereate to address: %v", err)
		}
	}
outerLoop:
	for len(utxoChannel) < targetUtxos {
		var rootTxOut *processor.TxOut
		rootTxOut, err = p.getCoinbaseTxOut()
		if err != nil {
			return err
		}

		p.logger.Debug("Splittable output", "hash", rootTxOut.Hash.String(), "value", rootTxOut.ValueSat)

		rootTx, err := p.splitToAddress(rootTxOut, outputsPerTx)
		if err != nil {
			return fmt.Errorf("failed split to address: %v", err)
		}

		hexString, err := getHexString(rootTx)
		if err != nil {
			return err
		}

		var sentTxHash *string
		sentTxHash, err = p.client.SendRawTransaction(hexString)
		if err != nil {
			if strings.Contains(err.Error(), "mandatory-script-verify-flag-failed") {
				p.logger.Error("Failed to send root tx", "err", err)

				continue
			}

			return fmt.Errorf("failed to send root tx: %v", err)
		}

		p.logger.Debug("Sent root tx", "hash", *sentTxHash, "outputs", len(rootTx.TxOut))

		hash := rootTx.TxHash()

		var splitTxOut *processor.TxOut

		for rootIndex, rootOutput := range rootTx.TxOut {
			splitTxOut = &processor.TxOut{
				Hash:            &hash,
				ValueSat:        rootOutput.Value,
				ScriptPubKeyHex: hex.EncodeToString(rootOutput.PkScript),
				VOut:            uint32(rootIndex),
			}

			splitTx1, err := p.splitToAddress(splitTxOut, outputsPerTx)
			if err != nil {
				continue
			}

			hexString, err := getHexString(rootTx)
			if err != nil {
				return err
			}
			sentTxHash, err = p.client.SendRawTransaction(hexString)
			if err != nil {
				return fmt.Errorf("failed to send splitTx1 tx: %v", err)
			}

			p.logger.Debug("Sent split tx", "hash", splitTx1.TxID(), "outputs", len(splitTx1.TxOut))
			for index, output := range splitTx1.TxOut {
				if len(utxoChannel) >= targetUtxos {
					break outerLoop
				}

				hash, err := chainhash.NewHashFromStr(*sentTxHash)
				if err != nil {
					return err
				}

				utxoChannel <- processor.TxOut{
					Hash:            hash,
					ScriptPubKeyHex: hex.EncodeToString(output.PkScript),
					ValueSat:        output.Value,
					VOut:            uint32(index),
				}
			}
		}
	}

	bhs, err := p.client.GenerateToAddress(1, p.address.EncodeAddress())
	if err != nil {
		return fmt.Errorf("failed to gnereate to address: %v", err)
	}

	p.logger.Info("Generated new block", "hash", bhs[0])

	close(signalFinish)
	<-loggingStopped
	p.logger.Info("Created utxos", slog.Int("count", len(utxoChannel)), slog.Int("target", targetUtxos))

	return nil
}

func (p *Client) splitToAddress(txOut *processor.TxOut, outputs int) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(wire.TxVersion)

	prevOut := wire.NewOutPoint(txOut.Hash, txOut.VOut)
	input := wire.NewTxIn(prevOut, nil, nil)
	tx.AddTxIn(input)

	pkScript, err := txscript.PayToAddrScript(p.address)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	sigScript, err := txscript.SignTxOutput(&chaincfg.RegressionNetParams,
		tx, 0, pkScriptOrig, txscript.SigHashAll,
		txscript.KeyClosure(lookupKey), nil, nil)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	return tx, nil
}

func (p *Client) GenerateBlock() (blockHash string, err error) {
	blockHashes, err := p.client.GenerateToAddress(1, p.address.EncodeAddress())
	if err != nil {
		return "", err
	}

	return blockHashes[0], nil
}
