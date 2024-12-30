package bsv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/boecklim/node-analysis/node_client"
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

	var responseResult T

	err = json.Unmarshal(rpcResponse.Result, &responseResult)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarhsal response: %v", err)
	}

	return &responseResult, nil
}

type Client struct {
	host     string
	port     int
	user     string
	password string

	logger *slog.Logger
}

func New(host string, port int, user, password string, logger *slog.Logger) (*Client, error) {
	c := &Client{
		logger:   logger,
		host:     host,
		port:     port,
		user:     user,
		password: password,
	}

	return c, nil
}

func (c *Client) SendRawTransaction(hexString string) (*string, error) {
	return sendJsonRPCCall[string]("sendrawtransaction", []interface{}{hexString, true, true}, c.host, c.port, c.user, c.password)
}

func (c *Client) GetMiningInfo() (*node_client.GetMiningInfoResult, error) {
	return sendJsonRPCCall[node_client.GetMiningInfoResult]("getmininginfo", nil, c.host, c.port, c.user, c.password)
}

func (c *Client) GetBlock(blockHash string) (*node_client.GetBlockVerboseResult, error) {
	return sendJsonRPCCall[node_client.GetBlockVerboseResult]("getblock", []interface{}{blockHash}, c.host, c.port, c.user, c.password)
}

func (c *Client) GetBlockHash(blockHeight int64) (*string, error) {
	return sendJsonRPCCall[string]("getblockhash", []interface{}{blockHeight}, c.host, c.port, c.user, c.password)
}

func (c *Client) GetTxOut(txHash string, index uint32, mempool bool) (*node_client.GetTxOutResult, error) {
	return sendJsonRPCCall[node_client.GetTxOutResult]("gettxout", []interface{}{txHash, index, mempool}, c.host, c.port, c.user, c.password)
}

func (c *Client) GetNetworkInfo() (*node_client.GetNetworkInfoResult, error) {
	return sendJsonRPCCall[node_client.GetNetworkInfoResult]("getnetworkinfo", nil, c.host, c.port, c.user, c.password)
}

func (c *Client) GenerateToAddress(nBlocks int64, address string) ([]string, error) {
	hashes, err := sendJsonRPCCall[[]string]("generatetoaddress", []interface{}{nBlocks, address}, c.host, c.port, c.user, c.password)
	if err != nil {
		return nil, err
	}

	return *hashes, nil
}

func (c *Client) GetRawMempool() ([]string, error) {
	hashes, err := sendJsonRPCCall[[]string]("getrawmempool", nil, c.host, c.port, c.user, c.password)
	if err != nil {
		return nil, err
	}

	return *hashes, nil
}
