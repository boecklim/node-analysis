package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"node-analysis/zmq"
	"os"

	"github.com/lmittmann/tint"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("failed to run listener: %v", err)
	}

	os.Exit(0)
}

const (
	host     = "localhost"
	user     = "bitcoin"
	password = "bitcoin"
	rpcPort  = 18443
	zmqPort  = 29000
)

func run() error {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	zmqURLString := fmt.Sprintf("zmq://%s:%d", host, zmqPort)

	zmqURL, err := url.Parse(zmqURLString)
	if err != nil {
		return fmt.Errorf("failed to parse zmq URL: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zmqSubscriber, err := zmq.NewZMQWithContext(ctx, host, zmqPort, logger)
	if err != nil {
		return err
	}

	zmqClient := zmq.NewZMQClient(zmqURL, logger)

	err = zmqClient.Start(ctx, zmqSubscriber)
	if err != nil {
		return err
	}

	return nil
}
