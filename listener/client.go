package listener

import (
	"context"
	"io"
	"log/slog"
	"node-analysis/broadcaster"
	"os"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lmittmann/tint"
	slogmulti "github.com/samber/slog-multi"
)

const (
	pubhashblock = "hashblock"
	pubhashtx    = "hashtx"
)

type Client struct {
	rpcClient broadcaster.RPCClient
}

func New(rpcClient broadcaster.RPCClient) *Client {
	z := &Client{
		rpcClient: rpcClient,
	}

	return z
}

type ClientI interface {
	Subscribe(string, chan []string) error
}

func (z *Client) Start(ctx context.Context, ignoreBlockHashes map[string]struct{}, ch chan []string, logFile io.Writer) {

	go func() {
		listenerLogger := slog.New(
			slogmulti.Fanout(
				slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}),
				tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelInfo, TimeFormat: time.Kitchen}),
			),
		)
		for {
			select {
			case <-ctx.Done():
				return

			case c := <-ch:
				switch c[0] {
				case pubhashblock:

					_, found := ignoreBlockHashes[c[1]]
					if found {
						continue
					}

					hash := c[1]

					timeStamp := time.Now()

					blockHash, err := chainhash.NewHashFromStr(hash)
					if err != nil {
						listenerLogger.Error("failed to create hash from hex string", "err", err)
						continue
					}

					sizeBytes, nrTxs, err := z.rpcClient.GetBlockSize(blockHash)
					if err != nil {
						listenerLogger.Error("failed to get block for block hash", "hash", blockHash.String(), "err", err)
						continue
					}

					listenerLogger.Info("block", "hash", hash, "timestamp", timeStamp.Format(time.RFC3339), "txs", nrTxs, "size", sizeBytes)

				default:
					listenerLogger.Warn("Unhandled ZMQ message", "msg", strings.Join(c, ","))
				}
			}
		}
	}()

}
