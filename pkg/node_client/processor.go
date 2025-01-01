package node_client

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/boecklim/node-analysis/pkg/processor"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

const (
	coinBaseVout    = 0
	satPerBtc       = 1e8
	outputsPerTx    = 20
	fee             = 3000
	blocksGenerated = 200
)

var _ processor.Processor = &Processor{}

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
	//Warnings        StringOrArray          `json:"warnings"`
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
	GetNetworkInfo() (*GetNetworkInfoResult, error)
	GetBlock(blockHash string) (*GetBlockVerboseResult, error)
	GetBlockHash(blockHeight int64) (*string, error)
	GetTxOut(txHash string, index uint32, mempool bool) (*GetTxOutResult, error)
	SendRawTransaction(hexString string, isBSV bool) (*string, error)
	GetRawMempool() ([]string, error)
}

type Processor struct {
	client RPCClient
	logger *slog.Logger
	isBSV  bool
	//pkScript []byte

	splitToAddressFunc func(txOut *processor.TxOut, outputs int) (res *splitResult, err error)
	//createSelfPayingTxFunc func(txOut *processor.TxOut) (*selfPayingResult, error)

	//privKeyBSV *bec.PrivateKey

	addressString string
	//address btcutil.Address
	privKey *btcec.PrivateKey
}

func (p *Processor) setAddress() error {
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

	//p.address = address
	p.privKey = privKey
	p.addressString = address.EncodeAddress()

	p.logger.Info("New address", "address", p.addressString)

	//pkScript, err := txscript.PayToAddrScript(p.address)
	//if err != nil {
	//	return err
	//}

	//p.pkScript = pkScript
	return nil
}

func NewProcessor(client RPCClient, logger *slog.Logger, isBSV bool) (*Processor, error) {
	p := &Processor{
		client: client,
		logger: logger,
		isBSV:  isBSV,
	}

	err := p.setAddress()
	if err != nil {
		return nil, err
	}
	if isBSV {
		//err := p.setAddressBSV()
		//if err != nil {
		//	return nil, err
		//}
		p.splitToAddressFunc = p.splitToAddressBSV
		//p.createSelfPayingTxFunc = p.createSelfPayingTxBSV
	} else {
		p.splitToAddressFunc = p.splitToAddressBTC
		//p.createSelfPayingTxFunc = p.createSelfPayingTxBTC
	}

	return p, nil
}

func (p *Processor) getCoinbaseTxOut() (*processor.TxOut, error) {
	var txOut *GetTxOutResult
	var txHash string

	var counter int64 = 0

	// Find a coinbase tx out which has not been spent yet
	for {
		if counter > 10 {
			return nil, errors.New("failed to find coinbase tx out")
		}

		currentBlockHeight, err := p.getBlockHeight()
		if err != nil {
			return nil, fmt.Errorf("failed to get info: %v", err)
		}

		randomHeightOfGeneratedBlock := currentBlockHeight - blocksGenerated + int64(rand.Intn(100))
		blockHash, err := p.client.GetBlockHash(randomHeightOfGeneratedBlock)
		if err != nil {
			return nil, fmt.Errorf("failed go get block hash at height %d: %v", randomHeightOfGeneratedBlock, err)
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

func (p *Processor) getBlockHeight() (int64, error) {
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

func (p *Processor) SubmitSelfPayingSingleOutputTx(txOut processor.TxOut) (txHash *chainhash.Hash, satoshis int64, err error) {
	txResult, err := p.splitToAddressFunc(&txOut, 0)
	if err != nil {
		return nil, 0, err
	}

	_, err = p.client.SendRawTransaction(txResult.hexString, p.isBSV)
	if err != nil {
		if strings.Contains(err.Error(), "Transaction outputs already in utxo set") {
			p.logger.Error("Submitting tx failed", "txOut.hash", txOut.Hash.String(), "txOut.value", txOut.ValueSat, "txOut.vout", txOut.VOut, "hash", txResult.hash.String(), "err", err)
		}
		return nil, 0, err
	}

	return txResult.hash, txResult.outputs[0].satoshis, nil
}

type selfPayingResult struct {
	hash      *chainhash.Hash
	satoshis  int64
	hexString string
}

func (p *Processor) GetBlockSize(blockHash *chainhash.Hash) (sizeBytes uint64, nrTxs uint64, err error) {
	blockMsg, err := p.client.GetBlock(blockHash.String())
	if err != nil {
		return 0, 0, err
	}

	return uint64(blockMsg.Size), uint64(len(blockMsg.Tx)), nil
}

func (p *Processor) GetMempoolSize() (nrTxs uint64, err error) {
	rawMempool, err := p.client.GetRawMempool()
	if err != nil {
		return 0, err
	}

	return uint64(len(rawMempool)), nil
}

func (p *Processor) PrepareUtxos(utxoChannel chan processor.TxOut, targetUtxos int) (err error) {
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

	_, err = p.client.GenerateToAddress(blocksGenerated, p.addressString)
	if err != nil {
		return fmt.Errorf("failed to gnereate to address: %v", err)
	}
outerLoop:
	for len(utxoChannel) < targetUtxos {
		var rootTxOut *processor.TxOut
		rootTxOut, err = p.getCoinbaseTxOut()
		if err != nil {
			return err
		}

		p.logger.Debug("Splittable output", "hash", rootTxOut.Hash.String(), "value", rootTxOut.ValueSat)

		rootSplitResult, err := p.splitToAddressFunc(rootTxOut, outputsPerTx)
		if err != nil {
			p.logger.Error("failed to split to address", "err", err)
			continue
		}

		var sentTxHash *string
		sentTxHash, err = p.client.SendRawTransaction(rootSplitResult.hexString, p.isBSV)
		if err != nil {
			if strings.Contains(err.Error(), "mandatory-script-verify-flag-failed") {
				p.logger.Error("Failed to send root tx", "err", err)

				continue
			}

			return fmt.Errorf("failed to send root tx: %v", err)
		}

		p.logger.Debug("Sent root tx", "hash", *sentTxHash, "outputs", len(rootSplitResult.outputs))

		var splitTxOut *processor.TxOut

		for rootIndex, rootOutput := range rootSplitResult.outputs {
			splitTxOut = &processor.TxOut{
				Hash:            rootSplitResult.hash,
				ValueSat:        rootOutput.satoshis,
				ScriptPubKeyHex: rootOutput.pkScript,
				VOut:            uint32(rootIndex),
			}

			splitTxSplitResult, err := p.splitToAddressFunc(splitTxOut, outputsPerTx)
			if err != nil {
				p.logger.Error("failed to split to address", "err", err)
				continue
			}

			splitTxHash, err := p.client.SendRawTransaction(splitTxSplitResult.hexString, p.isBSV)
			if err != nil {
				return fmt.Errorf("failed to send splitTx1 tx: %v", err)
			}

			p.logger.Debug("Sent split tx", "hash", splitTxSplitResult.hash.String(), "outputs", len(splitTxSplitResult.outputs))
			for index, output := range splitTxSplitResult.outputs {
				if len(utxoChannel) >= targetUtxos {
					break outerLoop
				}

				splitTxHashString, err := chainhash.NewHashFromStr(*splitTxHash)
				if err != nil {
					return err
				}

				utxoChannel <- processor.TxOut{
					Hash:            splitTxHashString,
					ScriptPubKeyHex: output.pkScript,
					ValueSat:        output.satoshis,
					VOut:            uint32(index),
				}
			}
		}
	}

	bhs, err := p.client.GenerateToAddress(1, p.addressString)
	if err != nil {
		return fmt.Errorf("failed to gnereate to address: %v", err)
	}

	p.logger.Info("Generated new block", "hash", bhs[0])

	close(signalFinish)
	<-loggingStopped
	p.logger.Info("Created utxos", slog.Int("count", len(utxoChannel)), slog.Int("target", targetUtxos))

	return nil
}

type splitOutput struct {
	pkScript string
	satoshis int64
}

type splitResult struct {
	outputs   []splitOutput
	hexString string
	hash      *chainhash.Hash
}

func (p *Processor) GenerateBlock() (blockHash string, err error) {
	blockHashes, err := p.client.GenerateToAddress(1, p.addressString)
	if err != nil {
		return "", err
	}

	return blockHashes[0], nil
}
