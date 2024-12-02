package main

import (
	"context"
	"fmt"
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

	publisher := NewRedisPublisher(redisAddr, streamName)
	ctx = context.Background()

	n := 5 // Number of frames to publish
	fmt.Println("Publishing frames...")
	for i := 0; i < n; i++ {
		err := publisher.Publish(ctx, map[string]interface{}{
			"frame_path": imagePath,
			"timestamp":  time.Now().Format(time.RFC3339),
		})
		if err != nil {
			fmt.Printf("Failed to publish frame %d: %v\n", i, err)
		}
		time.Sleep(500 * time.Millisecond) // Simulate a delay between frame publishing
	}
	fmt.Println("Finished publishing frames.")

	fmt.Println("Processing frames...")
	consumer := NewRedisConsumer(redisAddr, streamName, "camera_group", "consumer1")

	consumer.ProcessMessages(ctx, func(msgID string, values map[string]interface{}) error {
		framePath, ok := values["frame_path"].(string)
		if !ok {
			return fmt.Errorf("frame_path not found or invalid in message %s", msgID)
		}

		if err := RunModel(modelSession, framePath); err != nil {
			return fmt.Errorf("failed to process frame: %v", err)
		}

		fmt.Printf("Processed frame %s from message %s\n", framePath, msgID)
		return nil
	})

}
