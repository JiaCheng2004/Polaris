package openrouter

import (
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/contracttest"
)

func TestOpenrouterChatContract(t *testing.T) {
	contracttest.RunOpenAICompatChatSuite(t, "openrouter/auto", "auto",
		func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
			return NewChatAdapter(NewClient(cfg), model)
		})
}
