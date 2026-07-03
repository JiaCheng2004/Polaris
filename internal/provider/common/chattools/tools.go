package chattools

import (
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func OpenAITools(tools []modality.ToolDefinition) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		switch strings.TrimSpace(tool.Type) {
		case "", "function":
			if strings.TrimSpace(tool.Function.Name) == "" {
				continue
			}
			out = append(out, map[string]any{
				"type":     "function",
				"function": tool.Function,
			})
		case "hosted":
			if hosted := OpenAIHostedTool(tool.Hosted); hosted != nil {
				out = append(out, hosted)
			}
		}
	}
	return out
}

func OpenAIHostedTool(spec *modality.HostedToolSpec) map[string]any {
	if spec == nil {
		return nil
	}
	config := copyConfig(spec.Config)
	switch strings.TrimSpace(spec.Name) {
	case "web_search":
		config["type"] = "web_search"
	case "code_interpreter":
		config["type"] = "code_interpreter"
		if _, ok := config["container"]; !ok {
			config["container"] = "auto"
		}
	case "computer_use":
		config["type"] = "computer_use_preview"
	case "file_search":
		config["type"] = "file_search"
	case "image_generation":
		config["type"] = "image_generation"
	case "mcp":
		config["type"] = "mcp"
	case "url_context":
		config["type"] = "web_search"
	default:
		return nil
	}
	return config
}

func copyConfig(config map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range config {
		out[key] = value
	}
	return out
}
