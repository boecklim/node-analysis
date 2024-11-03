package zmq

import (
	"log/slog"
	"net/url"
	"strings"
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

	const hashtxTopic = "hashtx2"
	const invalidTxTopic = "invalidtx"
	const discardedFromMempoolTopic = "discardedfrommempool"

	const pubhashblock = "pubhashblock"
	const pubhashtx = "pubhashtx"
	const pubrawblock = "pubrawblock"
	const pubrawtx = "pubrawtx"

	go func() {
		for c := range ch {
			switch c[0] {
			case pubhashblock:
				z.logger.Debug(pubhashblock, slog.String("hash", c[1]))
			case pubhashtx:
				z.logger.Debug(pubhashtx, slog.String("hash", c[1]))
			case pubrawblock:
				z.logger.Debug(pubrawblock, slog.String("hash", c[1]))
			case pubrawtx:
				z.logger.Debug(pubrawtx, slog.String("hash", c[1]))
			default:
				z.logger.Info("Unhandled ZMQ message", slog.String("msg", strings.Join(c, ",")))
			}
		}
	}()

	if err := zmqi.Subscribe(hashtxTopic, ch); err != nil {
		return err
	}

	if err := zmqi.Subscribe(invalidTxTopic, ch); err != nil {
		return err
	}

	if err := zmqi.Subscribe(discardedFromMempoolTopic, ch); err != nil {
		return err
	}

	return nil
}
