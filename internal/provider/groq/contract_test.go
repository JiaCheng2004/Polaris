package groq

import (
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/contracttest"
)

func TestGroqChatContract(t *testing.T) {
	contracttest.RunOpenAICompatChatSuite(t, "groq/llama-3.3-70b", "llama-3.3-70b",
		func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
			return NewChatAdapter(NewClient(cfg), model)
		})
}
