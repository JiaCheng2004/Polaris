package bytedance

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

const (
	bytedanceListVoicesAction  = "ListBigModelTTSTimbres"
	bytedanceListVoicesVersion = "2025-05-20"
)

type VoiceCatalogAdapter struct {
	client *Client
}

type listBigModelTTSTimbresResponse struct {
	ResponseMetadata struct {
		RequestID string `json:"RequestId,omitempty"`
	} `json:"ResponseMetadata"`
	Result struct {
		Timbres []bigModelTimbre `json:"Timbres"`
	} `json:"Result"`
}

type bigModelTimbre struct {
	SpeakerID   string               `json:"SpeakerID"`
	TimbreInfos []bigModelTimbreInfo `json:"TimbreInfos"`
}

type bigModelTimbreInfo struct {
	SpeakerName string                  `json:"SpeakerName"`
	Gender      string                  `json:"Gender"`
	Age         string                  `json:"Age"`
	Categories  []bigModelCategory      `json:"Categories"`
	Emotions    []bigModelEmotionSample `json:"Emotions"`
}

type bigModelCategory struct {
	Category string `json:"Category"`
}

type bigModelEmotionSample struct {
	Emotion     string `json:"Emotion"`
	EmotionType string `json:"EmotionType"`
	DemoURL     string `json:"DemoURL"`
	DemoText    string `json:"DemoText"`
}

func NewVoiceCatalogAdapter(client *Client) *VoiceCatalogAdapter {
	return &VoiceCatalogAdapter{client: client}
}

func (a *VoiceCatalogAdapter) ListVoices(ctx context.Context, req *modality.VoiceCatalogRequest) (*modality.VoiceCatalogResponse, error) {
	if strings.EqualFold(req.Type, "custom") {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_voice_type", "type", "Provider-backed ByteDance voice listing currently supports built-in voices only.")
	}

	var parsed listBigModelTTSTimbresResponse
	if err := a.client.speechControlJSON(ctx, bytedanceListVoicesAction, bytedanceListVoicesVersion, map[string]any{}, &parsed); err != nil {
		return nil, err
	}

	allowed := make(map[string]struct{}, len(req.ConfiguredVoiceIDs))
	for _, voiceID := range req.ConfiguredVoiceIDs {
		trimmed := strings.TrimSpace(voiceID)
		if trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}

	items := make([]modality.VoiceCatalogItem, 0, len(parsed.Result.Timbres))
	for _, timbre := range parsed.Result.Timbres {
		if len(allowed) > 0 {
			if _, ok := allowed[timbre.SpeakerID]; !ok {
				continue
			}
		}
		item := modality.VoiceCatalogItem{
			ID:       timbre.SpeakerID,
			Provider: "bytedance",
			Type:     "builtin",
		}
		if len(req.ConfiguredVoiceIDs) > 0 && strings.TrimSpace(req.Model) != "" {
			item.Models = []string{req.Model}
		}
		if len(timbre.TimbreInfos) > 0 {
			info := timbre.TimbreInfos[0]
			item.Name = strings.TrimSpace(info.SpeakerName)
			item.Gender = strings.TrimSpace(info.Gender)
			item.Age = strings.TrimSpace(info.Age)
			item.Categories = compactCategories(info.Categories)
			item.Emotions = compactEmotions(info.Emotions)
			if len(item.Emotions) > 0 {
				item.PreviewURL = item.Emotions[0].PreviewURL
				item.PreviewText = item.Emotions[0].PreviewText
			}
		}
		items = append(items, item)
	}

	slices.SortFunc(items, func(aItem, bItem modality.VoiceCatalogItem) int {
		return strings.Compare(aItem.ID, bItem.ID)
	})
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}

	return &modality.VoiceCatalogResponse{
		Object:   "list",
		Scope:    "provider",
		Provider: "bytedance",
		Data:     items,
	}, nil
}

func compactCategories(categories []bigModelCategory) []string {
	items := make([]string, 0, len(categories))
	seen := map[string]struct{}{}
	for _, category := range categories {
		value := strings.TrimSpace(category.Category)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}
	return items
}

func compactEmotions(samples []bigModelEmotionSample) []modality.VoiceCatalogStyle {
	items := make([]modality.VoiceCatalogStyle, 0, len(samples))
	for _, sample := range samples {
		name := strings.TrimSpace(sample.Emotion)
		if name == "" {
			continue
		}
		items = append(items, modality.VoiceCatalogStyle{
			Name:        name,
			Type:        strings.TrimSpace(sample.EmotionType),
			PreviewURL:  strings.TrimSpace(sample.DemoURL),
			PreviewText: strings.TrimSpace(sample.DemoText),
		})
	}
	return items
}
