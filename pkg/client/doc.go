// Package client is the official Go SDK for the Polaris AI gateway.
//
// Polaris presents a single, OpenAI-compatible API across many providers and
// modalities — chat, responses, messages, embeddings, images, audio, video,
// music, files, and realtime — plus a control plane for projects, virtual keys,
// policies, budgets, tools, and MCP bindings. This package is a thin, typed
// client over that HTTP surface.
//
// # Getting started
//
// Construct a [Client] with a base URL and an API key, then call a method:
//
//	c, err := client.New("https://gateway.example.com", client.WithAPIKey("sk-..."))
//	if err != nil {
//		log.Fatal(err)
//	}
//	resp, err := c.CreateChatCompletion(ctx, &client.ChatCompletionRequest{
//		Model:    "openai/gpt-4o",
//		Messages: []client.ChatMessage{{Role: "user", Content: client.NewTextContent("Hello!")}},
//	})
//
// Model identifiers are always "provider/model" (for example "openai/gpt-4o",
// "anthropic/claude-sonnet-4-6") or a gateway-configured alias.
//
// # Options
//
// [New] accepts functional options: [WithAPIKey] sets the bearer token,
// [WithTimeout] overrides the default one-minute timeout, and [WithHTTPClient]
// supplies a custom *http.Client (for proxies, transport tuning, or tracing).
//
// # Streaming
//
// Streaming methods return an iterator with Next, Chunk, Err, and Close:
//
//	stream, err := c.StreamChatCompletion(ctx, req)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer stream.Close()
//	for stream.Next() {
//		fmt.Print(stream.Chunk().Choices[0].Delta.Content)
//	}
//	if err := stream.Err(); err != nil {
//		log.Fatal(err)
//	}
//
// Responses, messages, and music streams follow the same shape. Realtime
// (WebSocket) surfaces — transcription, translation, and interpreting — expose
// Dial*Session helpers that return a connection with Send, Receive, and Close.
//
// # Errors
//
// API failures are returned as [*APIError], which carries the OpenAI-compatible
// error type, code, message, and HTTP status so callers can branch on them.
package client
