package zmq

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
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

func (z *Client) Start(ctx context.Context, zmqi ClientI) error {
	ch := make(chan []string)

	if err := zmqi.Subscribe(pubhashblock, ch); err != nil {
		return err
	}

	if err := zmqi.Subscribe(pubhashtx, ch); err != nil {
		return err
	}

loop:
	for {
		select {
		case <-ctx.Done():
			break loop

		case c := <-ch:
			switch c[0] {
			case pubhashblock:
				z.logger.Debug("ZMQ", "topic", pubhashblock, "hash", c[1])
			case pubhashtx:
				z.logger.Debug("ZMQ", "topic", pubhashtx, "hash", c[1])
			default:
				z.logger.Info("Unhandled ZMQ message", "msg", strings.Join(c, ","))
			}
		}
	}

	return nil
}
