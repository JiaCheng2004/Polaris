package middleware

type pricing struct {
	inputPerMillion  float64
	outputPerMillion float64
}

var knownPricing = map[string]pricing{
	"openai/gpt-4o":      {inputPerMillion: 5.00, outputPerMillion: 15.00},
	"openai/gpt-4o-mini": {inputPerMillion: 0.15, outputPerMillion: 0.60},
	"openai/text-embedding-3-small": {
		inputPerMillion: 0.02,
	},
	"openai/text-embedding-3-large": {
		inputPerMillion: 0.13,
	},
	"anthropic/claude-sonnet-4-6": {inputPerMillion: 3.00, outputPerMillion: 15.00},
	"anthropic/claude-opus-4-6":   {inputPerMillion: 15.00, outputPerMillion: 75.00},
}

func EstimateCostUSD(model string, promptTokens int, completionTokens int) float64 {
	price, ok := knownPricing[model]
	if !ok {
		return 0
	}
	return (float64(promptTokens)*price.inputPerMillion + float64(completionTokens)*price.outputPerMillion) / 1_000_000
}
