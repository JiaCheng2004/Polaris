package moonshot

import (
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/contracttest"
)

func TestMoonshotChatContract(t *testing.T) {
	contracttest.RunOpenAICompatChatSuite(t, "moonshot/moonshot-v1-8k", "moonshot-v1-8k",
		func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
			return NewChatAdapter(NewClient(cfg), model)
		})
}
