package zmq

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

const (
	pubhashblock = "hashblock"
	pubhashtx    = "hashtx"
)

type Client struct {
	url    *url.URL
	logger *slog.Logger
}

func NewZMQClient(zmqURL *url.URL, logger *slog.Logger) *Client {
	z := &Client{
		url:    zmqURL,
		logger: logger,
	}

	return z
}

type ClientI interface {
	Subscribe(string, chan []string) error
}

type BlockEvent struct {
	Hash      string
	Timestamp time.Time
}

func (z *Client) Start(ctx context.Context, zmqi ClientI, blockChan chan BlockEvent, ch chan []string) error {

loop:
	for {
		select {
		case <-ctx.Done():
			break loop

		case c := <-ch:
			switch c[0] {
			case pubhashblock:
				blockChan <- BlockEvent{
					Hash:      c[1],
					Timestamp: time.Now(),
				}
				// z.logger.Debug("ZMQ", "topic", pubhashblock, "hash", c[1])
			case pubhashtx:
				// z.logger.Debug("ZMQ", "topic", pubhashtx, "hash", c[1])
			default:
				z.logger.Info("Unhandled ZMQ message", "msg", strings.Join(c, ","))
			}
		}
	}

	return nil
}
