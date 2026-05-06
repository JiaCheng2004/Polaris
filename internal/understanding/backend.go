package understanding

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	defaultRemoteProcessorTimeout = 30 * time.Second
	defaultChunkChars             = 1800
	defaultChunkOverlapChars      = 200
)

type RemoteProcessorConfig struct {
	Name            string
	Version         string
	Backend         string
	Endpoint        string
	Method          string
	Headers         map[string]string
	MIMETypes       []string
	FileClasses     []FileClass
	ArtifactKinds   []ArtifactKind
	Capabilities    []ProcessorCapability
	Profiles        []ProcessingProfile
	Timeout         time.Duration
	Priority        int
	RequiresNetwork bool
	Description     string
	Notes           []string
}

type TikaProcessorConfig struct {
	Name      string
	Version   string
	Endpoint  string
	Headers   map[string]string
	MIMETypes []string
	Profiles  []ProcessingProfile
	Timeout   time.Duration
	Priority  int
	OCR       bool
}

type ChunkOptions struct {
	Enabled      bool
	MaxChars     int
	OverlapChars int
}

type PipelineOptions struct {
	PlanOptions
	Chunking ChunkOptions
}

type PipelineResult struct {
	Plan      *Plan
	Artifacts []Artifact
}

type RemoteProcessor struct {
	cfg    RemoteProcessorConfig
	client *http.Client
}

type TikaProcessor struct {
	cfg    TikaProcessorConfig
	client *http.Client
}

type ChunkProcessor struct {
	MaxChars     int
	OverlapChars int
}

type remoteProcessorRequest struct {
	FileID       string     `json:"file_id,omitempty"`
	Sha256       string     `json:"sha256,omitempty"`
	Filename     string     `json:"filename,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	Size         int64      `json:"size,omitempty"`
	DataBase64   string     `json:"data_base64,omitempty"`
	DerivedText  string     `json:"derived_text,omitempty"`
	MaxTextChars int        `json:"max_text_chars,omitempty"`
	Artifacts    []Artifact `json:"artifacts,omitempty"`
}

type remoteProcessorResponse struct {
	Artifact  *Artifact      `json:"artifact,omitempty"`
	Artifacts []Artifact     `json:"artifacts,omitempty"`
	Text      string         `json:"text,omitempty"`
	Kind      ArtifactKind   `json:"kind,omitempty"`
	Warning   string         `json:"warning,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func NewRegistryWithRemoteProcessors(processors ...Processor) *Registry {
	base := NewDefaultRegistry()
	base.processors = append(base.processors, processors...)
	return base
}

func NewRemoteProcessor(cfg RemoteProcessorConfig) (*RemoteProcessor, error) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	if cfg.Name == "" {
		return nil, fmt.Errorf("remote processor name is required")
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("remote processor endpoint is required")
	}
	if _, err := url.ParseRequestURI(cfg.Endpoint); err != nil {
		return nil, fmt.Errorf("remote processor endpoint: %w", err)
	}
	if cfg.Version == "" {
		cfg.Version = "v1"
	}
	if cfg.Backend == "" {
		cfg.Backend = "remote_http"
	}
	if cfg.Method == "" {
		cfg.Method = http.MethodPost
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultRemoteProcessorTimeout
	}
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = []ProcessingProfile{ProfileBalanced, ProfileQuality}
	}
	if len(cfg.ArtifactKinds) == 0 {
		cfg.ArtifactKinds = []ArtifactKind{ArtifactText}
	}
	if len(cfg.Capabilities) == 0 {
		cfg.Capabilities = []ProcessorCapability{CapabilityTextExtract}
	}
	return &RemoteProcessor{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, nil
}

func (p *RemoteProcessor) Name() string {
	return p.cfg.Name
}

func (p *RemoteProcessor) Version() string {
	return p.cfg.Version
}

func (p *RemoteProcessor) Supports(mimeType string) bool {
	return processorSupportsMIME(mimeType, p.cfg.MIMETypes, p.cfg.FileClasses)
}

func (p *RemoteProcessor) Descriptor() ProcessorDescriptor {
	return ProcessorDescriptor{
		Name:            p.Name(),
		Version:         p.Version(),
		Backend:         p.cfg.Backend,
		Description:     firstString(p.cfg.Description, "Calls an operator-configured HTTP file-understanding processor."),
		FileClasses:     append([]FileClass(nil), p.cfg.FileClasses...),
		MIMETypes:       append([]string(nil), p.cfg.MIMETypes...),
		ArtifactKinds:   append([]ArtifactKind(nil), p.cfg.ArtifactKinds...),
		Capabilities:    append([]ProcessorCapability(nil), p.cfg.Capabilities...),
		Profiles:        append([]ProcessingProfile(nil), p.cfg.Profiles...),
		LatencyTier:     LatencyRemote,
		CostTier:        CostMedium,
		Deterministic:   false,
		Lossy:           true,
		RequiresNetwork: true,
		Priority:        p.cfg.Priority,
		Notes:           append([]string(nil), p.cfg.Notes...),
	}
}

func (p *RemoteProcessor) Process(ctx context.Context, input Input) (*Artifact, error) {
	reqBody := remoteProcessorRequest{
		FileID:       input.FileID,
		Sha256:       input.Sha256,
		Filename:     input.Filename,
		MimeType:     input.MimeType,
		Size:         input.Size,
		DataBase64:   base64.StdEncoding.EncodeToString(input.Data),
		DerivedText:  input.DerivedText,
		MaxTextChars: input.MaxTextChars,
		Artifacts:    append([]Artifact(nil), input.Artifacts...),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, p.cfg.Method, p.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for key, value := range p.cfg.Headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("remote processor %s returned %d: %s", p.Name(), resp.StatusCode, strings.TrimSpace(string(limited)))
	}
	var decoded remoteProcessorResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&decoded); err != nil {
		return nil, err
	}
	artifact := decoded.Artifact
	if artifact == nil && len(decoded.Artifacts) > 0 {
		artifact = &decoded.Artifacts[0]
	}
	if artifact == nil {
		artifact = &Artifact{
			Kind:     decoded.Kind,
			Text:     decoded.Text,
			Warning:  decoded.Warning,
			Metadata: decoded.Metadata,
		}
	}
	if artifact.Kind == "" && len(p.cfg.ArtifactKinds) > 0 {
		artifact.Kind = p.cfg.ArtifactKinds[0]
	}
	if artifact.Metadata == nil {
		artifact.Metadata = map[string]any{}
	}
	artifact.Metadata["backend"] = p.cfg.Backend
	artifact.Metadata["remote_endpoint"] = redactEndpoint(p.cfg.Endpoint)
	return artifact, nil
}

func NewTikaProcessor(cfg TikaProcessorConfig) (*TikaProcessor, error) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	if cfg.Name == "" {
		cfg.Name = "tika_text_extract"
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("tika endpoint is required")
	}
	if _, err := url.ParseRequestURI(cfg.Endpoint); err != nil {
		return nil, fmt.Errorf("tika endpoint: %w", err)
	}
	if cfg.Version == "" {
		cfg.Version = "v1"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultRemoteProcessorTimeout
	}
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = []ProcessingProfile{ProfileBalanced, ProfileQuality}
	}
	return &TikaProcessor{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, nil
}

func (p *TikaProcessor) Name() string {
	return p.cfg.Name
}

func (p *TikaProcessor) Version() string {
	return p.cfg.Version
}

func (p *TikaProcessor) Supports(mimeType string) bool {
	return processorSupportsMIME(mimeType, p.cfg.MIMETypes, nil)
}

func (p *TikaProcessor) Descriptor() ProcessorDescriptor {
	caps := []ProcessorCapability{CapabilityTextExtract, CapabilityMetadata}
	artifacts := []ArtifactKind{ArtifactText, ArtifactMetadata}
	if p.cfg.OCR {
		caps = append(caps, CapabilityOCR)
		artifacts = append(artifacts, ArtifactOCRText)
	}
	return ProcessorDescriptor{
		Name:            p.Name(),
		Version:         p.Version(),
		Backend:         "tika_server",
		Description:     "Calls Apache Tika Server for broad document text extraction and optional OCR.",
		FileClasses:     []FileClass{FileClassText, FileClassPDF, FileClassOfficeDocument, FileClassOfficeSpreadsheet, FileClassOfficePresentation, FileClassImage},
		MIMETypes:       append([]string(nil), p.cfg.MIMETypes...),
		ArtifactKinds:   artifacts,
		Capabilities:    caps,
		Profiles:        append([]ProcessingProfile(nil), p.cfg.Profiles...),
		LatencyTier:     LatencyRemote,
		CostTier:        CostLow,
		Deterministic:   true,
		Lossy:           true,
		RequiresNetwork: true,
		Priority:        p.cfg.Priority,
		Notes:           []string{"Tika quality depends on the operator's configured parsers and OCR runtime."},
	}
}

func (p *TikaProcessor) Process(ctx context.Context, input Input) (*Artifact, error) {
	endpoint := tikaTextEndpoint(p.cfg.Endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(input.Data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", firstString(input.MimeType, "application/octet-stream"))
	req.Header.Set("Accept", "text/plain")
	for key, value := range p.cfg.Headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("tika processor returned %d: %s", resp.StatusCode, strings.TrimSpace(string(limited)))
	}
	textBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	text := normalizeSourceText(string(textBytes))
	metadata := map[string]any{
		"artifact_kind":     string(ArtifactText),
		"backend":           "tika_server",
		"file_class":        string(ClassifyMIME(input.MimeType)),
		"representation":    "tika_text",
		"source_preserving": true,
	}
	if p.cfg.OCR {
		metadata["ocr_enabled"] = true
	}
	return &Artifact{
		Kind:     ArtifactText,
		Text:     text,
		Warning:  WarningQualityDegraded + " Tika extraction quality depends on configured parsers, OCR language packs, and source document structure.",
		Metadata: metadata,
	}, nil
}

func (p ChunkProcessor) Name() string {
	return "polaris_recursive_chunker"
}

func (p ChunkProcessor) Version() string {
	return "v1"
}

func (p ChunkProcessor) Supports(mimeType string) bool {
	return strings.TrimSpace(mimeType) != ""
}

func (p ChunkProcessor) Descriptor() ProcessorDescriptor {
	return ProcessorDescriptor{
		Name:          p.Name(),
		Version:       p.Version(),
		Backend:       "polaris_builtin",
		Description:   "Splits extracted text into overlapping source-preserving chunks for retrieval or embedding.",
		FileClasses:   []FileClass{FileClassText, FileClassPDF, FileClassOfficeDocument, FileClassOfficeSpreadsheet, FileClassOfficePresentation},
		MIMETypes:     []string{"*/*"},
		ArtifactKinds: []ArtifactKind{ArtifactChunk},
		Capabilities:  []ProcessorCapability{CapabilityChunk},
		Profiles:      []ProcessingProfile{ProfileFast, ProfileBalanced, ProfileQuality},
		LatencyTier:   LatencyLocalFast,
		CostTier:      CostFree,
		Deterministic: true,
		Lossy:         false,
		Priority:      100,
	}
}

func (p ChunkProcessor) Process(_ context.Context, input Input) (*Artifact, error) {
	text := firstString(input.DerivedText, firstTextArtifact(input.Artifacts))
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("chunk processor requires derived text")
	}
	maxChars := p.MaxChars
	if maxChars <= 0 {
		maxChars = defaultChunkChars
	}
	overlap := p.OverlapChars
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxChars {
		overlap = maxChars / 5
	}
	chunks := chunkText(text, maxChars, overlap)
	for i := range chunks {
		chunks[i].ID = fmt.Sprintf("chunk_%04d", i+1)
		chunks[i].Source = Provenance{
			FileID:       input.FileID,
			Sha256:       input.Sha256,
			Filename:     input.Filename,
			MimeType:     input.MimeType,
			Processor:    p.Name(),
			ArtifactKind: string(ArtifactChunk),
			StartChar:    chunks[i].StartChar,
			EndChar:      chunks[i].EndChar,
		}
	}
	metadata := map[string]any{
		"artifact_kind":       string(ArtifactChunk),
		"chunk_count":         len(chunks),
		"chunk_max_chars":     maxChars,
		"chunk_overlap_chars": overlap,
		"source_preserving":   true,
		"representation":      "recursive_text_chunks",
	}
	return &Artifact{
		Kind:     ArtifactChunk,
		Text:     formatChunksForContext(chunks),
		Metadata: metadata,
		Chunks:   chunks,
		Warning:  WarningQualityDegraded + " Text chunks preserve extracted text spans, but chunk boundaries are derived for retrieval and may not match original page or layout boundaries.",
	}, nil
}

func ProcessPipeline(ctx context.Context, registry *Registry, input Input, opts PipelineOptions) (*PipelineResult, error) {
	if registry == nil {
		registry = NewDefaultRegistry()
	}
	plan, err := registry.Plan(input, opts.PlanOptions)
	if err != nil {
		return nil, err
	}
	primary := plan.PrimaryProcessor()
	if primary == nil {
		return nil, ErrUnsupportedMIME{MIMEType: input.MimeType}
	}
	primaryArtifact, err := ProcessWithProcessor(ctx, primary, input)
	if err != nil {
		return nil, err
	}
	artifacts := []Artifact{*primaryArtifact}
	if opts.Chunking.Enabled && strings.TrimSpace(primaryArtifact.Text) != "" {
		chunker := ChunkProcessor{MaxChars: opts.Chunking.MaxChars, OverlapChars: opts.Chunking.OverlapChars}
		chunkInput := input
		chunkInput.DerivedText = primaryArtifact.Text
		chunkInput.Artifacts = artifacts
		if chunkArtifact, err := ProcessWithProcessor(ctx, chunker, chunkInput); err == nil {
			artifacts = append(artifacts, *chunkArtifact)
		}
	}
	return &PipelineResult{Plan: plan, Artifacts: artifacts}, nil
}

func processorSupportsMIME(mimeType string, mimeTypes []string, classes []FileClass) bool {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	if mimeType == "" {
		return false
	}
	if len(mimeTypes) == 0 && len(classes) == 0 {
		return true
	}
	for _, candidate := range mimeTypes {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		switch {
		case candidate == "*/*" || candidate == mimeType:
			return true
		case strings.HasSuffix(candidate, "/*") && strings.HasPrefix(mimeType, strings.TrimSuffix(candidate, "*")):
			return true
		}
	}
	class := ClassifyMIME(mimeType)
	for _, candidate := range classes {
		if candidate == class {
			return true
		}
	}
	return false
}

func firstTextArtifact(artifacts []Artifact) string {
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Text) != "" {
			return artifact.Text
		}
	}
	return ""
}

func chunkText(text string, maxChars int, overlap int) []Chunk {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	var chunks []Chunk
	for start := 0; start < len(runes); {
		end := start + maxChars
		if end >= len(runes) {
			end = len(runes)
		} else if boundary := bestChunkBoundary(runes, start, end); boundary > start {
			end = boundary
		}
		chunkRunes := runes[start:end]
		chunks = append(chunks, Chunk{
			Text:          strings.TrimSpace(string(chunkRunes)),
			StartChar:     start,
			EndChar:       end,
			TokenEstimate: estimateTokensFromRunes(chunkRunes),
		})
		if end >= len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return chunks
}

func bestChunkBoundary(runes []rune, start int, end int) int {
	min := start + (end-start)/2
	for _, sep := range []string{"\n\n", "\n", ". ", " "} {
		sepRunes := []rune(sep)
		for i := end - len(sepRunes); i >= min; i-- {
			if runeSliceHasPrefix(runes[i:end], sepRunes) {
				return i + len(sepRunes)
			}
		}
	}
	return end
}

func runeSliceHasPrefix(value []rune, prefix []rune) bool {
	if len(value) < len(prefix) {
		return false
	}
	for i := range prefix {
		if value[i] != prefix[i] {
			return false
		}
	}
	return true
}

func estimateTokensFromRunes(runes []rune) int {
	if len(runes) == 0 {
		return 0
	}
	return (len(runes) + 3) / 4
}

func formatChunksForContext(chunks []Chunk) string {
	var b strings.Builder
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.Text) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[")
		b.WriteString(chunk.ID)
		b.WriteString(" chars ")
		fmt.Fprintf(&b, "%d-%d", chunk.StartChar, chunk.EndChar)
		b.WriteString("]\n")
		b.WriteString(chunk.Text)
	}
	return b.String()
}

func tikaTextEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	if strings.HasSuffix(parsed.Path, "/tika") {
		return parsed.String()
	}
	parsed.Path = path.Join(parsed.Path, "tika")
	return parsed.String()
}

func redactEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
