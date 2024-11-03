package zmq

import (
	"log/slog"
	"net/url"
	"strings"
)

const (
	pubhashblock = "hashblock"
	pubhashtx    = "hashtx"
)

type ZMQClient struct {
	url    *url.URL
	logger *slog.Logger
}

func NewZMQClient(zmqURL *url.URL, logger *slog.Logger) *ZMQClient {
	z := &ZMQClient{
		url:    zmqURL,
		logger: logger,
	}

	return z
}

type ZMQI interface {
	Subscribe(string, chan []string) error
}

func (z *ZMQClient) Start(zmqi ZMQI) error {
	ch := make(chan []string)

	go func() {
		for c := range ch {

			// z.logger.Debug("zmq", slog.String("hash", c[1]))

			switch c[0] {
			case pubhashblock:
				z.logger.Debug("ZMQ", "topic", pubhashblock, "hash", c[1])
			case pubhashtx:
				z.logger.Debug("ZMQ", "topic", pubhashtx, "hash", c[1])
			default:
				z.logger.Info("Unhandled ZMQ message", "msg", strings.Join(c, ","))
			}
		}
	}()

	if err := zmqi.Subscribe(pubhashblock, ch); err != nil {
		return err
	}

	if err := zmqi.Subscribe(pubhashtx, ch); err != nil {
		return err
	}

	return nil
}
