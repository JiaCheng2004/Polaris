package bytedance

import "strings"

var bytedanceProviderModelAliases = map[string]string{
	"doubao-seed-2.0-pro":      "doubao-seed-2-0-pro-260215",
	"doubao-seed-2.0-lite":     "doubao-seed-2-0-lite-260215",
	"doubao-seed-2.0-mini":     "doubao-seed-2-0-mini-260215",
	"doubao-seed-1.6-vision":   "doubao-seed-1-6-vision-250815",
	"doubao-seedream-5.0-lite": "doubao-seedream-5-0-lite-260128",
	"doubao-seedream-4.5":      "doubao-seedream-4.5",
	"doubao-seedance-2.0":      "doubao-seedance-2-0-260128",
	"doubao-seedance-2.0-fast": "doubao-seedance-2-0-fast-260128",
	"seedream-5.0-lite":        "doubao-seedream-5-0-lite-260128",
	"seedream-4.5":             "doubao-seedream-4.5",
	"seedance-2.0":             "doubao-seedance-2-0-260128",
	"seedance-2.0-fast":        "doubao-seedance-2-0-fast-260128",
	"seed-2.0-pro":             "doubao-seed-2-0-pro-260215",
	"seed-2.0-lite":            "doubao-seed-2-0-lite-260215",
	"seed-2.0-mini":            "doubao-seed-2-0-mini-260215",
	"seed-1.6-vision":          "doubao-seed-1-6-vision-250815",
}

var bytedanceStreamingASRResourceAliases = map[string]string{
	"doubao-streaming-asr-2.0":            "volc.seedasr.sauc.duration",
	"doubao-streaming-asr-2.0-concurrent": "volc.seedasr.sauc.concurrent",
	"doubao-streaming-asr-1.0":            "volc.bigasr.sauc.duration",
	"doubao-streaming-asr-1.0-concurrent": "volc.bigasr.sauc.concurrent",
	"doubao-asr-streaming-2.0":            "volc.seedasr.sauc.duration",
	"doubao-asr-streaming-2.0-concurrent": "volc.seedasr.sauc.concurrent",
}

func providerChatModelName(requestModel string, fallbackModel string) string {
	return resolveBytedanceModelAlias(providerModelName(requestModel, fallbackModel))
}

func providerImageModelName(requestModel string, fallbackModel string) string {
	return resolveBytedanceModelAlias(providerModelName(requestModel, fallbackModel))
}

func providerVideoModelName(requestModel string, fallbackModel string) string {
	return resolveBytedanceModelAlias(providerModelName(requestModel, fallbackModel))
}

func resolveBytedanceModelAlias(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if mapped, ok := bytedanceProviderModelAliases[normalized]; ok {
		return mapped
	}
	return name
}

func providerStreamingASRResourceID(requestModel string, fallbackModel string) string {
	name := strings.ToLower(strings.TrimSpace(providerModelName(requestModel, fallbackModel)))
	if mapped, ok := bytedanceStreamingASRResourceAliases[name]; ok {
		return mapped
	}
	return "volc.seedasr.sauc.duration"
}
