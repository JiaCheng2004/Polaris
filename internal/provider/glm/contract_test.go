package glm

import (
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/contracttest"
)

func TestGlmChatContract(t *testing.T) {
	contracttest.RunOpenAICompatChatSuite(t, "glm/glm-4", "glm-4",
		func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
			return NewChatAdapter(NewClient(cfg), model)
		})
}
