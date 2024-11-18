package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"node-analysis/zmq"
	"os"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("failed to run listener: %v", err)
	}

	os.Exit(0)
}

const (
	hostDefault    = "localhost"
	zmqPortDefault = 29000
)

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	zmqPort := flag.Int("port", zmqPortDefault, "port of listener client")
	if zmqPort == nil {
		return errors.New("rpc port not given")
	}

	zmqHost := flag.String("host", hostDefault, "host of listener client")
	if zmqHost == nil {
		return errors.New("rpc host not given")
	}

	flag.Parse()

	zmqURLString := fmt.Sprintf("zmq://%s:%d", *zmqHost, *zmqPort)

	zmqURL, err := url.Parse(zmqURLString)
	if err != nil {
		return fmt.Errorf("failed to parse zmq URL: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zmqSubscriber, err := zmq.NewZMQWithContext(ctx, *zmqHost, *zmqPort, logger)
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
