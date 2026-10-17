package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName = "scheduler.exchange"
	QueueName    = "task.dispatch.queue"
	RoutingKey   = "task.dispatch"
)

type Client struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	confirms chan amqp.Confirmation
}

type DispatchMessage struct {
	RunID string `json:"run_id"`
}

func NewClient(amqpURL string) (*Client, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	if err := ch.Confirm(false); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to put channel in confirm mode: %w", err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 100))

	err = ch.ExchangeDeclare(
		ExchangeName,
		"direct",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	queue, err := ch.QueueDeclare(
		QueueName,
		true,  // durable
		false, // auto-deleted
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	err = ch.QueueBind(
		queue.Name,
		RoutingKey,
		ExchangeName,
		false,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	return &Client{
		conn:     conn,
		channel:  ch,
		confirms: confirms,
	}, nil
}
func (c *Client) PublishWithConfirm(ctx context.Context, msg DispatchMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal dispatch message: %w", err)
	}

	err = c.channel.PublishWithContext(
		ctx,
		ExchangeName,
		RoutingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now().UTC(),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	select {
	case confirmed := <-c.confirms:
		if !confirmed.Ack {
			return fmt.Errorf("message delivery nacked by broker")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) Close() error {
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) StartConsumer(ctx context.Context) (<-chan amqp.Delivery, error) {
	err := c.channel.Qos(
		10,    // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set channel qos: %w", err)
	}

	deliveries, err := c.channel.ConsumeWithContext(
		ctx,
		QueueName,
		"task-dispatcher-worker", // consumer tag
		false,                    // autoAck: false
		false,                    // exclusive
		false,                    // noLocal
		false,                    // noWait
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register consumer: %w", err)
	}

	return deliveries, nil
}
