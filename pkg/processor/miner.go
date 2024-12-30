package processor

import (
	"context"
	"log/slog"
	"math"
	"math/rand"
	"time"
)

type Client struct {
	client   Processor
	shutdown chan struct{}
}

// NewMiner creates a new simulated miner
func NewMiner(client Processor) *Client {
	c := &Client{
		client:   client,
		shutdown: make(chan struct{}, 1),
	}

	return c
}

func randomSampleExpDist(tau time.Duration) time.Duration {
	lambda := 1 / float64(tau.Milliseconds())

	interval := -1 * math.Log(rand.Float64()) / lambda

	return time.Duration(interval) * time.Millisecond
}

func (c *Client) Start(ctx context.Context, genBlocksInterval time.Duration, newBlockChan chan string, logger *slog.Logger, startAt time.Time) {
	logger = logger.With(slog.String("service", "miner"))

	durationUntilNextBlockMined := randomSampleExpDist(genBlocksInterval)

	timer := time.NewTimer(durationUntilNextBlockMined)

	startTimer := time.NewTimer(time.Until(startAt))
	logger.Info("Waiting to start", "until", startAt.String())
	<-startTimer.C

	go func() {
		defer func() {
			logger.Info("stopping broadcasting")
		}()

		for {
			select {
			case blockHash := <-newBlockChan: // A block has been found by another miner -> reset the timer
				durationUntilNextBlockMined = randomSampleExpDist(genBlocksInterval)
				logger.Info("Block found", "hash", blockHash, slog.String("next block", durationUntilNextBlockMined.String()))

				timer.Reset(durationUntilNextBlockMined)
			case <-timer.C: // time is up -> miner has found a block
				blockHash, err := c.client.GenerateBlock()
				if err != nil {
					logger.Error("failed to generate block", "err", err)
					continue
				}

				logger.Info("Block generated", "hash", blockHash)
			case <-ctx.Done():
				return
			}
		}
	}()
}
