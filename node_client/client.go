package node_client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/boecklim/node-analysis/processor"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type RPCRequest struct {
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int64       `json:"id"`
	JSONRpc string      `json:"jsonrpc"`
}

type RPCResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Err    interface{}     `json:"error"`
}

func sendJsonRPCCall[T any](method string, params []interface{}, nodeHost string, nodePort int, nodeUser, nodePassword string) (*T, error) {
	c := http.Client{}

	rpcRequest := RPCRequest{method, params, time.Now().UnixNano(), "1.0"}
	payloadBuffer := &bytes.Buffer{}
	jsonEncoder := json.NewEncoder(payloadBuffer)

	err := jsonEncoder.Encode(rpcRequest)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("%s://%s:%d", "http", nodeHost, nodePort),
		payloadBuffer,
	)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(nodeUser, nodePassword)
	req.Header.Add("Content-Type", "application/json;charset=utf-8")
	req.Header.Add("Accept", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rpcResponse RPCResponse

	if resp.StatusCode != 200 {
		_ = json.Unmarshal(data, &rpcResponse)
		v, ok := rpcResponse.Err.(map[string]interface{})
		if ok {
			err = errors.New(v["message"].(string))
		} else {
			err = errors.New("HTTP error: " + resp.Status)
		}
		if err != nil {
			return nil, err
		}
	}

	err = json.Unmarshal(data, &rpcResponse)
	if err != nil {
		return nil, err
	}

	if rpcResponse.Err != nil {
		e, ok := rpcResponse.Err.(error)
		if ok {
			return nil, e
		}
		return nil, errors.New("unknown error returned from node in rpc response")
	}

	var finalResp T

	err = json.Unmarshal(rpcResponse.Result, &finalResp)
	if err != nil {
		return nil, err
	}

	return &finalResp, nil
}

type Client struct {
	host     string
	port     int
	user     string
	passowrd string

	logger   *slog.Logger
	pkScript []byte
	address  btcutil.Address
	privKey  *btcec.PrivateKey
}

func (c *Client) setAddress() error {
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

	c.address = address
	c.privKey = privKey

	c.logger.Info("New address", "address", address.EncodeAddress())

	pkScript, err := txscript.PayToAddrScript(c.address)
	if err != nil {
		return err
	}

	c.pkScript = pkScript
	return nil
}

func (c *Client) PrepareUtxos(utxoChannel chan processor.TxOut, targetUtxos int) (err error) {
	//TODO implement me
	panic("implement me")
}

func (c *Client) SubmitSelfPayingSingleOutputTx(txOut processor.TxOut) (txHash *chainhash.Hash, satoshis int64, err error) {
	//TODO implement me
	panic("implement me")
}

func (c *Client) GenerateBlock() ([]string, error) {
	blockHashes, err := sendJsonRPCCall[[]string]("generatetoaddress", []interface{}{1, c.address.EncodeAddress(), 3}, c.host, c.port, c.user, c.passowrd)
	if err != nil {
		return nil, err
	}

	return *blockHashes, nil
}

func (c *Client) GetBlockSize(blockHash *chainhash.Hash) (sizeBytes uint64, nrTxs uint64, err error) {
	//TODO implement me
	panic("implement me")
}

func (c *Client) GetMempoolSize() (nrTxs uint64, err error) {
	//TODO implement me
	panic("implement me")
}

func New(host string, port int, user, password string, logger *slog.Logger) (*Client, error) {
	c := &Client{
		logger:   logger,
		host:     host,
		port:     port,
		user:     user,
		passowrd: password,
	}

	err := c.setAddress()
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) GetMiningInfo() (*GetMiningInfoResult, error) {
	return sendJsonRPCCall[GetMiningInfoResult]("getmininginfo", nil, c.host, c.port, c.user, c.passowrd)
}

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
