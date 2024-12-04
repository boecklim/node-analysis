package zmq

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-zeromq/zmq4"
)

// var allowedTopics = []string{
// 	"hashblock",
// 	"hashblock2",
// 	"hashtx",
// 	"hashtx2",
// 	"rawblock",
// 	"rawblock2",
// 	"rawtx",
// 	"rawtx2",
// 	"discardedfrommempool",
// 	"removedfrommempoolblock",
// 	"invalidtx",
// }

type subscriptionRequest struct {
	topic string
	ch    chan []string
}

// ZMQ struct
type ZMQ struct {
	address            string
	socket             zmq4.Socket
	connected          bool
	err                error
	subscriptions      map[string][]chan []string
	addSubscription    chan subscriptionRequest
	removeSubscription chan subscriptionRequest
	logger             *slog.Logger
}

func NewZMQ(host string, port int, logger *slog.Logger) (*ZMQ, error) {
	ctx := context.Background()

	return NewZMQWithContext(ctx, host, port, logger)
}

func NewZMQWithContext(ctx context.Context, host string, port int, logger *slog.Logger) (*ZMQ, error) {
	zmq := &ZMQ{
		address:            fmt.Sprintf("tcp://%s:%d", host, port),
		subscriptions:      make(map[string][]chan []string),
		addSubscription:    make(chan subscriptionRequest, 10),
		removeSubscription: make(chan subscriptionRequest, 10),
		logger:             logger,
		socket:             zmq4.NewSub(ctx, zmq4.WithID(zmq4.SocketIdentity("sub"))),
	}

	err := zmq.dial(ctx)
	if err != nil {
		return nil, err
	}

	return zmq, nil
}

func (zmq *ZMQ) dial(ctx context.Context) error {
	err := zmq.socket.Dial(zmq.address)
	if err == nil {
		return nil
	}

	ticker := time.NewTicker(5 * time.Second)
	counter := 0
dialLoop:
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if counter >= 5 {
				return errors.New("failed to connect to ZMQ after 5 retries")
			}

			err := zmq.socket.Dial(zmq.address)
			if err != nil {
				zmq.err = err
				zmq.logger.Error(fmt.Sprintf("Could not dial ZMQ at %s: %v", zmq.address, err))
				zmq.logger.Info("Attempting to re-establish ZMQ connection in 10 seconds...")
				counter++
				continue
			}

			break dialLoop
		}
	}

	zmq.logger.Info(fmt.Sprintf("ZMQ: Connecting to %s", zmq.address))

	for topic := range zmq.subscriptions {
		if err := zmq.socket.SetOption(zmq4.OptionSubscribe, topic); err != nil {
			return err
		}
		zmq.logger.Info(fmt.Sprintf("ZMQ: Subscribed to %s", topic))
	}

	return nil
}

func (zmq *ZMQ) Subscribe(topic string, ch chan []string) error {
	// if !contains(allowedTopics, topic) {
	// 	return fmt.Errorf("topic must be %+v, received %q", allowedTopics, topic)
	// }

	zmq.addSubscription <- subscriptionRequest{
		topic: topic,
		ch:    ch,
	}

	return nil
}

func (zmq *ZMQ) Unsubscribe(topic string, ch chan []string) error {
	// if !contains(allowedTopics, topic) {
	// 	return fmt.Errorf("topic must be %+v, received %q", allowedTopics, topic)
	// }

	zmq.removeSubscription <- subscriptionRequest{
		topic: topic,
		ch:    ch,
	}

	return nil
}

func (zmq *ZMQ) Start(ctx context.Context) error {
	for topic := range zmq.subscriptions {
		if err := zmq.socket.SetOption(zmq4.OptionSubscribe, topic); err != nil {
			return err
		}
		zmq.logger.Info(fmt.Sprintf("ZMQ: Subscribed to %s", topic))
	}

	go func() {
		for {
		OUT:
			for {
				select {
				case <-ctx.Done():
					zmq.logger.Info("ZMQ: Context done, exiting")
					return
				case req := <-zmq.addSubscription:
					if err := zmq.socket.SetOption(zmq4.OptionSubscribe, req.topic); err != nil {
						zmq.logger.Error(fmt.Sprintf("ZMQ: Failed to subscribe to %s", req.topic))
					} else {
						zmq.logger.Info(fmt.Sprintf("ZMQ: Subscribed to %s", req.topic))
					}

					subscribers := zmq.subscriptions[req.topic]
					subscribers = append(subscribers, req.ch)

					zmq.subscriptions[req.topic] = subscribers

				case req := <-zmq.removeSubscription:
					subscribers := zmq.subscriptions[req.topic]
					for i, subscriber := range subscribers {
						if subscriber == req.ch {
							subscribers = append(subscribers[:i], subscribers[i+1:]...)
							zmq.logger.Info(fmt.Sprintf("Removed subscription from %s topic", req.topic))
							break
						}
					}
					zmq.subscriptions[req.topic] = subscribers

				default:
					msg, err := zmq.socket.Recv()
					if err != nil {
						if errors.Is(err, context.Canceled) {
							return
						}
						zmq.logger.Error(fmt.Sprintf("zmq.socket.Recv() - %v\n", err))
						break OUT
					} else {
						if !zmq.connected {
							zmq.connected = true
							zmq.logger.Info(fmt.Sprintf("ZMQ: Connection to %s observed\n", zmq.address))
						}

						subscribers := zmq.subscriptions[string(msg.Frames[0])]

						sequence := "N/A"

						if len(msg.Frames) > 2 && len(msg.Frames[2]) == 4 {
							s := binary.LittleEndian.Uint32(msg.Frames[2])
							sequence = strconv.FormatInt(int64(s), 10)
						}

						for _, subscriber := range subscribers {
							subscriber <- []string{string(msg.Frames[0]), hex.EncodeToString(msg.Frames[1]), sequence}
						}
					}
				}
			}

			if zmq.connected {
				zmq.socket.Close()
				zmq.connected = false
			}
			log.Printf("Attempting to re-establish ZMQ connection in 10 seconds...")
			time.Sleep(10 * time.Second)
		}
	}()

	return nil
}
