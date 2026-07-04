package fireworks

import (
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/contracttest"
)

func TestFireworksChatContract(t *testing.T) {
	contracttest.RunOpenAICompatChatSuite(t, "fireworks/accounts/fireworks/llama", "accounts/fireworks/llama",
		func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
			return NewChatAdapter(NewClient(cfg), model)
		})
}
