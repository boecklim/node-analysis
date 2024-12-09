package processor

import (
	"context"
	"log/slog"
	"math"
	"math/rand"
	"time"
)

type Client struct {
	client   RPCClient
	shutdown chan struct{}
	logger   *slog.Logger
}

// NewMiner creates a new simulated miner
func NewMiner(client RPCClient, logger *slog.Logger) *Client {
	c := &Client{
		client:   client,
		shutdown: make(chan struct{}, 1),
		logger:   logger,
	}

	return c
}

func randomSampleExpDist(tau time.Duration) time.Duration {
	lambda := 1 / float64(tau.Milliseconds())

	interval := -1 * math.Log(rand.Float64()) / lambda

	return time.Duration(interval) * time.Millisecond
}

func (c *Client) Start(ctx context.Context, genBlocksInterval time.Duration, newBlockChan chan string, startAt time.Time) {
	var err error
	var durationUntilNextBlockMined time.Duration

	durationUntilNextBlockMined = randomSampleExpDist(genBlocksInterval)

	timer := time.NewTimer(durationUntilNextBlockMined)

	var blockID string

	startTimer := time.NewTimer(time.Until(startAt))
	c.logger.Info("Waiting to start", "until", startAt.String())
	<-startTimer.C

	go func() {
		defer func() {
			c.logger.Info("stopping broadcasting")
		}()

		for {
			select {
			case blockHash := <-newBlockChan: // A block has been found by another miner -> reset the timer
				durationUntilNextBlockMined = randomSampleExpDist(genBlocksInterval)
				c.logger.Info("New block found", "hash", blockHash, slog.Duration("next block", durationUntilNextBlockMined))

				timer.Reset(durationUntilNextBlockMined)
			case <-timer.C: // time is up -> miner has found a block
				blockID, err = c.client.GenerateBlock()
				if err != nil {
					c.logger.Error("failed to generate block", "err", err)
					continue
				}

				c.logger.Info("Block generated", "ID", blockID)
			case <-ctx.Done():
				return
			}
		}
	}()
}
