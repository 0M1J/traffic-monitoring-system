package main

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

type RedisPublisher struct {
	client *redis.Client
	stream string
}

// NewRedisPublisher creates a new RedisPublisher for a given stream.
func NewRedisPublisher(redisAddr, stream string) *RedisPublisher {
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	return &RedisPublisher{
		client: client,
		stream: stream,
	}
}

// Publish sends data to the Redis stream.
func (p *RedisPublisher) Publish(ctx context.Context, values map[string]interface{}) error {
	err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: values,
	}).Err()
	if err != nil {
		log.Printf("Failed to publish to stream %s: %v", p.stream, err)
		return err
	}
	return nil
}

type RedisConsumer struct {
	client       *redis.Client
	stream       string
	group        string
	consumerName string
}

// NewRedisConsumer creates a new RedisConsumer for a given stream and consumer group.
func NewRedisConsumer(redisAddr, stream, group, consumerName string) *RedisConsumer {
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	return &RedisConsumer{
		client:       client,
		stream:       stream,
		group:        group,
		consumerName: consumerName,
	}
}

// ProcessMessages processes messages from the Redis stream using a custom handler function.
func (c *RedisConsumer) ProcessMessages(ctx context.Context, handler func(msgID string, values map[string]interface{}) error) {
	for {
		// Read messages from the stream
		messages, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumerName,
			Streams:  []string{c.stream, ">"},
			Count:    10,
			Block:    0, // Block until messages arrive
		}).Result()
		if err != nil {
			log.Printf("Error reading messages from stream %s: %v", c.stream, err)
			continue
		}

		for _, message := range messages[0].Messages {
			msgID := message.ID
			values := message.Values

			// Process the message using the provided handler
			if err := handler(msgID, values); err != nil {
				log.Printf("Error processing message %s: %v", msgID, err)
				continue
			}

			// Acknowledge the message
			if err := c.client.XAck(ctx, c.stream, c.group, msgID).Err(); err != nil {
				log.Printf("Failed to acknowledge message %s: %v", msgID, err)
			}
		}
	}
}
