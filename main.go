package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var modelSession *ModelSession

func init() {
	var err error
	modelSession, err = initSession()
	if err != nil {
		fmt.Printf("Failed to initialize session: %v\n", err)
		return
	}
}

func main() {
	defer modelSession.Destroy()

	imagePath := "car.png"

	redisAddr := "localhost:6379"
	streamName := "camera_stream"
	consumerGroup := "camera_group"
	numConsumers := 30  // Number of consumers
	numPublishers := 20 // Number of publishers

	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer client.Close()

	// Ensure consumer group exists
	ctx := context.Background()
	err := client.XGroupCreateMkStream(ctx, streamName, consumerGroup, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		fmt.Printf("Failed to create consumer group: %v\n", err)
		return
	}

	fmt.Println("Consumer group created or already exists.")

	consumerSemaphore := make(chan struct{}, numConsumers)
	publisherSemaphore := make(chan struct{}, numPublishers)

	var wg sync.WaitGroup

	publisher := NewRedisPublisher(redisAddr, streamName)
	for i := 0; i < numPublishers; i++ {
		wg.Add(1)
		go func(publisherName string) {
			defer wg.Done()
			publisherSemaphore <- struct{}{}        // Acquire a semaphore slot
			defer func() { <-publisherSemaphore }() // Release the semaphore slot

			// Mimic publishing frames
			n := 5 // Number of frames to publish
			fmt.Printf("Publisher %s starting...\n", publisherName)
			for i := 0; i < n; i++ {
				err := publisher.Publish(ctx, map[string]interface{}{
					"frame_path": imagePath,
					"timestamp":  time.Now().Format(time.RFC3339),
				})
				if err != nil {
					fmt.Printf("Publisher %s failed to publish frame %d: %v\n", publisherName, i, err)
				}
				time.Sleep(500 * time.Millisecond) // Simulate a delay between frame publishing
			}
			fmt.Printf("Publisher %s finished.\n", publisherName)
		}(fmt.Sprintf("publisher-%d", i+1))
	}

	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func(consumerName string) {
			defer wg.Done()
			consumerSemaphore <- struct{}{}        // Acquire a semaphore slot
			defer func() { <-consumerSemaphore }() // Release the semaphore slot

			consumer := NewRedisConsumer(redisAddr, streamName, consumerGroup, consumerName)

			// Process messages with the consumer
			consumer.ProcessMessages(ctx, func(msgID string, values map[string]interface{}) error {
				framePath, ok := values["frame_path"].(string)
				if !ok {
					return fmt.Errorf("frame_path not found or invalid in message %s", msgID)
				}

				if err := RunModel(modelSession, framePath); err != nil {
					fmt.Printf("Consumer %s error processing frame %s: %v\n", consumerName, framePath, err)
					return nil // Continue processing other frames even if this one fails
				}

				fmt.Printf("Consumer %s processed frame %s from message %s\n", consumerName, framePath, msgID)
				return nil
			})
		}(fmt.Sprintf("consumer-%d", i+1))
	}

	// Wait for all consumers and publishers to finish
	wg.Wait()

}
