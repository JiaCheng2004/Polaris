package provider

import (
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/bedrock"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/contracttest"
	"github.com/JiaCheng2004/Polaris/internal/provider/featherless"
	"github.com/JiaCheng2004/Polaris/internal/provider/fireworks"
	"github.com/JiaCheng2004/Polaris/internal/provider/glm"
	"github.com/JiaCheng2004/Polaris/internal/provider/groq"
	"github.com/JiaCheng2004/Polaris/internal/provider/mistral"
	"github.com/JiaCheng2004/Polaris/internal/provider/moonshot"
	"github.com/JiaCheng2004/Polaris/internal/provider/nvidia"
	"github.com/JiaCheng2004/Polaris/internal/provider/openrouter"
	"github.com/JiaCheng2004/Polaris/internal/provider/together"
)

func TestPhaseAOpenAICompatibleChatContracts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		canonicalModel    string
		expectedWireModel string
		factory           contracttest.OpenAICompatAdapterFactory
	}{
		{
			name:              "openrouter",
			canonicalModel:    "openrouter/openai/gpt-5.4-mini",
			expectedWireModel: "openai/gpt-5.4-mini",
			factory: func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
				return openrouter.NewChatAdapter(openrouter.NewClient(cfg), model)
			},
		},
		{
			name:              "together",
			canonicalModel:    "together/meta-llama/Llama-3.3-70B-Instruct-Turbo",
			expectedWireModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			factory: func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
				return together.NewChatAdapter(together.NewClient(cfg), model)
			},
		},
		{
			name:              "groq",
			canonicalModel:    "groq/llama-3.3-70b-versatile",
			expectedWireModel: "llama-3.3-70b-versatile",
			factory: func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
				return groq.NewChatAdapter(groq.NewClient(cfg), model)
			},
		},
		{
			name:              "fireworks",
			canonicalModel:    "fireworks/accounts/fireworks/models/llama-v3p1-8b-instruct",
			expectedWireModel: "accounts/fireworks/models/llama-v3p1-8b-instruct",
			factory: func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
				return fireworks.NewChatAdapter(fireworks.NewClient(cfg), model)
			},
		},
		{
			name:              "featherless",
			canonicalModel:    "featherless/meta-llama/Meta-Llama-3.1-8B-Instruct",
			expectedWireModel: "meta-llama/Meta-Llama-3.1-8B-Instruct",
			factory: func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
				return featherless.NewChatAdapter(featherless.NewClient(cfg), model)
			},
		},
		{
			name:              "nvidia",
			canonicalModel:    "nvidia/nvidia/NVIDIA-Nemotron-Nano-9B-v2",
			expectedWireModel: "nvidia/NVIDIA-Nemotron-Nano-9B-v2",
			factory: func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
				return nvidia.NewChatAdapter(nvidia.NewClient(cfg), model)
			},
		},
		{
			name:              "moonshot",
			canonicalModel:    "moonshot/kimi-k2-turbo-preview",
			expectedWireModel: "kimi-k2-turbo-preview",
			factory: func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
				return moonshot.NewChatAdapter(moonshot.NewClient(cfg), model)
			},
		},
		{
			name:              "glm",
			canonicalModel:    "glm/glm-5.1",
			expectedWireModel: "glm-5.1",
			factory: func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
				return glm.NewChatAdapter(glm.NewClient(cfg), model)
			},
		},
		{
			name:              "mistral",
			canonicalModel:    "mistral/mistral-medium-latest",
			expectedWireModel: "mistral-medium-latest",
			factory: func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
				return mistral.NewChatAdapter(mistral.NewClient(cfg), model)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			contracttest.RunOpenAICompatChatSuite(t, tc.canonicalModel, tc.expectedWireModel, tc.factory)
		})
	}
}

func TestPhaseANativeChatContracts(t *testing.T) {
	t.Parallel()

	contracttest.RunBedrockNativeChatSuite(
		t,
		"bedrock/amazon.nova-2-lite-v1:0",
		"amazon.nova-2-lite-v1:0",
		func(cfg config.ProviderConfig, model string) modality.ChatAdapter {
			return bedrock.NewChatAdapter(bedrock.NewClient(cfg), model)
		},
	)
}
