package client_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/JiaCheng2004/Polaris/pkg/client"
)

func ExampleNew() {
	c, err := client.New(
		"https://gateway.example.com",
		client.WithAPIKey("sk-polaris-..."),
		client.WithTimeout(30*time.Second),
		client.WithHTTPClient(&http.Client{}),
	)
	if err != nil {
		log.Fatal(err)
	}
	_ = c
}

func ExampleClient_CreateChatCompletion() {
	c, err := client.New("https://gateway.example.com", client.WithAPIKey("sk-polaris-..."))
	if err != nil {
		log.Fatal(err)
	}
	resp, err := c.CreateChatCompletion(context.Background(), &client.ChatCompletionRequest{
		Model: "openai/gpt-4o",
		Messages: []client.ChatMessage{
			{Role: "system", Content: client.NewTextContent("You are concise.")},
			{Role: "user", Content: client.NewTextContent("Say hello in one word.")},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	if text := resp.Choices[0].Message.Content.Text; text != nil {
		fmt.Println(*text)
	}
}

func ExampleClient_StreamChatCompletion() {
	c, err := client.New("https://gateway.example.com", client.WithAPIKey("sk-polaris-..."))
	if err != nil {
		log.Fatal(err)
	}
	stream, err := c.StreamChatCompletion(context.Background(), &client.ChatCompletionRequest{
		Model:    "anthropic/claude-sonnet-4-6",
		Messages: []client.ChatMessage{{Role: "user", Content: client.NewTextContent("Count to three.")}},
		Stream:   true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()
	for stream.Next() {
		for _, choice := range stream.Chunk().Choices {
			fmt.Print(choice.Delta.Content)
		}
	}
	if err := stream.Err(); err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_CreateEmbedding() {
	c, err := client.New("https://gateway.example.com", client.WithAPIKey("sk-polaris-..."))
	if err != nil {
		log.Fatal(err)
	}
	resp, err := c.CreateEmbedding(context.Background(), &client.EmbeddingRequest{
		Model: "openai/text-embedding-3-small",
		Input: client.NewMultiEmbeddingInput("first document", "second document"),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("received %d embeddings\n", len(resp.Data))
}

func ExampleClient_ListModels() {
	c, err := client.New("https://gateway.example.com", client.WithAPIKey("sk-polaris-..."))
	if err != nil {
		log.Fatal(err)
	}
	models, err := c.ListModels(context.Background(), true)
	if err != nil {
		log.Fatal(err)
	}
	for _, m := range models.Data {
		fmt.Println(m.ID)
	}
}

func ExampleClient_CreateProject() {
	c, err := client.New("https://gateway.example.com", client.WithAPIKey("sk-admin-..."))
	if err != nil {
		log.Fatal(err)
	}
	project, err := c.CreateProject(context.Background(), &client.CreateProjectRequest{
		Name:        "mobile-app",
		Description: "Production traffic for the mobile app.",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(project.ID)
}

// This example shows branching on a typed API error.
func ExampleAPIError() {
	c, err := client.New("https://gateway.example.com", client.WithAPIKey("sk-polaris-..."))
	if err != nil {
		log.Fatal(err)
	}
	_, err = c.CreateChatCompletion(context.Background(), &client.ChatCompletionRequest{
		Model:    "openai/does-not-exist",
		Messages: []client.ChatMessage{{Role: "user", Content: client.NewTextContent("hi")}},
	})
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		fmt.Printf("request failed: %s (%s, HTTP %d)\n", apiErr.Type, apiErr.Code, apiErr.StatusCode)
	}
}
