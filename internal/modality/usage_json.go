package modality

import "encoding/json"

func (u *Usage) UnmarshalJSON(data []byte) error {
	type usageAlias Usage
	var raw struct {
		usageAlias
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
		PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
		PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = Usage(raw.usageAlias)
	if raw.PromptTokensDetails != nil {
		u.CachedInputTokens = raw.PromptTokensDetails.CachedTokens
	}
	if raw.CompletionTokensDetails != nil {
		u.ReasoningTokens = raw.CompletionTokensDetails.ReasoningTokens
	}
	if raw.PromptCacheHitTokens > 0 {
		u.CachedInputTokens = raw.PromptCacheHitTokens
	}
	return nil
}
