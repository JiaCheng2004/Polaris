// Command stream is a runnable example: a streaming chat completion through a
// Polaris gateway, printing tokens as they arrive over SSE.
//
//	POLARIS_BASE_URL   gateway URL (default http://localhost:8080)
//	POLARIS_API_KEY    API key for the gateway
//	POLARIS_MODEL      model as "provider/model" (default openai/gpt-4o)
//
//	go run ./examples/go/stream
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/JiaCheng2004/Polaris/pkg/client"
)

func main() {
	c, err := client.New(getenv("POLARIS_BASE_URL", "http://localhost:8080"),
		client.WithAPIKey(os.Getenv("POLARIS_API_KEY")),
	)
	if err != nil {
		log.Fatalf("new client: %v", err)
	}

	stream, err := c.StreamChatCompletion(context.Background(), &client.ChatCompletionRequest{
		Model:    getenv("POLARIS_MODEL", "openai/gpt-4o"),
		Messages: []client.ChatMessage{{Role: "user", Content: client.NewTextContent("Count to five, one number per line.")}},
		Stream:   true,
	})
	if err != nil {
		log.Fatalf("stream: %v", err)
	}
	defer stream.Close()

	for stream.Next() {
		for _, choice := range stream.Chunk().Choices {
			fmt.Print(choice.Delta.Content)
		}
	}
	if err := stream.Err(); err != nil {
		log.Fatalf("stream error: %v", err)
	}
	fmt.Println()
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
