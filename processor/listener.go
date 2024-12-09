package processor

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
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

func (l *Listener) Start(ctx context.Context, messageChan chan []string, newBlockCh chan string, logger *slog.Logger, logAfter time.Time) {
	logger = logger.With(slog.String("service", "listener"))

	lastBlockFound := time.Now()
	go func() {

		for {
			select {
			case <-ctx.Done():
				return

			case c := <-messageChan:
				switch c[0] {
				case pubhashblock:
					if time.Now().Before(logAfter) {
						// Do not log anything before this point in time
						continue
					}

					hash := c[1]

					blockHash, err := chainhash.NewHashFromStr(hash)
					if err != nil {
						logger.Error("Failed to create hash from hex string", "err", err)
						continue
					}

					sizeBytes, nrTxs, err := l.rpcClient.GetBlockSize(blockHash)
					if err != nil {
						logger.Error("Failed to get block for block hash", "hash", blockHash.String(), "err", err)
						continue
					}

					timestamp := time.Now()
					timeSinceLastBlock := timestamp.Sub(lastBlockFound)
					logger.Info("Block", "hash", hash, "timestamp", timestamp.Format(time.RFC3339Nano), "delta", timeSinceLastBlock.String(), "txs", nrTxs, "size", sizeBytes)

					lastBlockFound = timestamp

					newBlockCh <- hash

				default:
					logger.Warn("Unhandled ZMQ message", "msg", strings.Join(c, ","))
				}
			}
		}
	}()
}
