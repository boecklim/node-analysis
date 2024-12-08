package processor

import (
	"context"
	"io"
	"log/slog"
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

type Listener struct {
	rpcClient RPCClient
}

func NewListener(rpcClient RPCClient) *Listener {
	l := &Listener{
		rpcClient: rpcClient,
	}

	return l
}

type ClientI interface {
	Subscribe(string, chan []string) error
}

func (l *Listener) Start(ctx context.Context, ignoreBlockHashes map[string]struct{}, messageChan chan []string, newBlockCh chan struct{}, logFile io.Writer) {
	lastBlockFound := time.Now()
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

			case c := <-messageChan:
				switch c[0] {
				case pubhashblock:
					_, found := ignoreBlockHashes[c[1]]
					if found {
						continue
					}

					hash := c[1]

					blockHash, err := chainhash.NewHashFromStr(hash)
					if err != nil {
						listenerLogger.Error("Failed to create hash from hex string", "err", err)
						continue
					}

					sizeBytes, nrTxs, err := l.rpcClient.GetBlockSize(blockHash)
					if err != nil {
						listenerLogger.Error("Failed to get block for block hash", "hash", blockHash.String(), "err", err)
						continue
					}

					// Todo: log time elapsed since last block
					timestamp := time.Now()
					timeSinceLastBlock := timestamp.Sub(lastBlockFound)
					listenerLogger.Info("Block", "hash", hash, "timestamp", timestamp.Format(time.RFC3339Nano), "delta", timeSinceLastBlock.String(), "txs", nrTxs, "size", sizeBytes)

					lastBlockFound = timestamp

					newBlockCh <- struct{}{}

				default:
					listenerLogger.Warn("Unhandled ZMQ message", "msg", strings.Join(c, ","))
				}
			}
		}
	}()

}
