package together

import (
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/contracttest"
)

func TestTogetherChatContract(t *testing.T) {
	contracttest.RunOpenAICompatChatSuite(t, "together/llama-3", "llama-3",
		func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
			return NewChatAdapter(NewClient(cfg), model)
		})
}
