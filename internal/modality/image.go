package modality

import "context"

type ImageAdapter interface {
	Generate(ctx context.Context, req *ImageRequest) (*ImageResponse, error)
	Edit(ctx context.Context, req *ImageEditRequest) (*ImageResponse, error)
}

type ImageRequest struct {
	Model           string          `json:"model"`
	Routing         *RoutingOptions `json:"routing,omitempty"`
	Prompt          string          `json:"prompt"`
	N               int             `json:"n,omitempty"`
	Size            string          `json:"size,omitempty"`
	Quality         string          `json:"quality,omitempty"`
	Style           string          `json:"style,omitempty"`
	ResponseFormat  string          `json:"response_format,omitempty"`
	ReferenceImages []string        `json:"reference_images,omitempty"`
}

type ImageEditRequest struct {
	Model          string          `json:"model"`
	Routing        *RoutingOptions `json:"routing,omitempty"`
	Prompt         string          `json:"prompt"`
	Image          []byte          `json:"-"`
	ImageFilename  string          `json:"-"`
	ImageType      string          `json:"-"`
	Mask           []byte          `json:"-"`
	MaskFilename   string          `json:"-"`
	MaskType       string          `json:"-"`
	N              int             `json:"n,omitempty"`
	Size           string          `json:"size,omitempty"`
	ResponseFormat string          `json:"response_format,omitempty"`
}

type ImageResponse struct {
	Created int64             `json:"created"`
	Data    []ImageResultData `json:"data"`
}

type ImageResultData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}
