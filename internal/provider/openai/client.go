package openai

import (
	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/openaicompat"
)

// Client wraps the shared OpenAI-compatible transport with OpenAI's defaults.
// Chat and embeddings run through the compat base; the unique OpenAI adapters
// (image, voice, video, files, batch, responses, realtime) reach the underlying
// http.Client and credentials via the embedded compat client's accessors.
type Client struct {
	*openaicompat.Client
}

// NewClient builds an OpenAI client on the shared transport core.
func NewClient(cfg config.ProviderConfig) *Client {
	return &Client{Client: openaicompat.NewClient("openai", "OpenAI", cfg, "https://api.openai.com/v1", nil)}
}

// The following unexported delegators keep the historical helper names used
// throughout the OpenAI adapter files while routing to the single shared
// implementation in the compat package.

func providerModelName(requestModel, fallbackModel string) string {
	return openaicompat.ProviderModelName(requestModel, fallbackModel)
}

func firstNonEmpty(values ...string) string {
	return openaicompat.FirstNonEmpty(values...)
}

func translateTransportError(err error, providerName string) error {
	return apierror.ProviderTransportError(err, providerName)
}
