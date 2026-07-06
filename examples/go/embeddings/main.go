// Command embeddings is a runnable example: request an embedding vector through
// a Polaris gateway using the Go SDK.
//
//	POLARIS_BASE_URL   gateway URL (default http://localhost:8080)
//	POLARIS_API_KEY    API key for the gateway
//	POLARIS_MODEL      model (default openai/text-embedding-3-small)
//
//	go run ./examples/go/embeddings
package main

import (
	"context"
	"fmt"
	"log"
	"os"

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
	)
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}

	resp, err := c.CreateEmbedding(context.Background(), &client.EmbeddingRequest{
		Model: getenv("POLARIS_MODEL", "openai/text-embedding-3-small"),
		Input: client.NewSingleEmbeddingInput("The quick brown fox jumps over the lazy dog."),
	})
	if err != nil {
		return fmt.Errorf("embedding: %w", err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("no embedding data returned")
	}
	vec := resp.Data[0].Embedding.Float32
	fmt.Printf("model %s returned a %d-dimension embedding; first values: %v...\n",
		resp.Model, len(vec), firstN(vec, 4))
	return nil
}

func firstN(v []float32, n int) []float32 {
	if len(v) < n {
		return v
	}
	return v[:n]
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
