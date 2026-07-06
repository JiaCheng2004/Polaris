// Command chat is a runnable example: a single chat completion through a Polaris
// gateway using the Go SDK. Configure it with the environment:
//
//	POLARIS_BASE_URL   gateway URL (default http://localhost:8080)
//	POLARIS_API_KEY    API key for the gateway
//	POLARIS_MODEL      model as "provider/model" (default openai/gpt-4o)
//
//	go run ./examples/go/chat
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/JiaCheng2004/Polaris/pkg/client"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	c, err := client.New(getenv("POLARIS_BASE_URL", "http://localhost:8080"),
		client.WithAPIKey(os.Getenv("POLARIS_API_KEY")),
		client.WithTimeout(60*time.Second),
	)
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := c.CreateChatCompletion(ctx, &client.ChatCompletionRequest{
		Model: getenv("POLARIS_MODEL", "openai/gpt-4o"),
		Messages: []client.ChatMessage{
			{Role: "system", Content: client.NewTextContent("You are concise.")},
			{Role: "user", Content: client.NewTextContent("Say hello in one word.")},
		},
	})
	if err != nil {
		return fmt.Errorf("chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return fmt.Errorf("no choices returned")
	}
	if text := resp.Choices[0].Message.Content.Text; text != nil {
		fmt.Println(*text)
	}
	fmt.Printf("[tokens in/out: %d/%d]\n", resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
