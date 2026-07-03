package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	commonfiles "github.com/JiaCheng2004/Polaris/internal/provider/common/files"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/transport"
	"github.com/JiaCheng2004/Polaris/internal/understanding"
	"github.com/gin-gonic/gin"
)

const fileUnderstandingModeDerived = "derived_context"

type chatFileUnderstandingPolicy struct {
	Enabled bool
	Mode    string
	Profile understanding.ProcessingProfile
	Config  config.FileUnderstandingConfig
}

type fileUnderstandingInput struct {
	FileID    string
	ProjectID string
	URL       string
	Sha256    string
	Filename  string
	MimeType  string
	Size      int64
	Data      []byte
	Source    string
}

func (h *ChatHandler) resolveFilesForTarget(c *gin.Context, req *modality.ChatRequest, model provider.Model) (*modality.ChatRequest, error) {
	if h.store == nil || !requestHasFileParts(req) {
		return req, nil
	}
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Config == nil {
		return nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "runtime_unavailable", "file_id", "Runtime configuration is unavailable.")
	}
	projectID := chatProjectID(middleware.GetAuthContext(c))
	resolver := commonfiles.Resolver{
		Store:    h.store,
		Registry: snapshot.Registry,
		Config:   snapshot.Config.Files,
	}
	understandingPolicy := fileUnderstandingPolicy(req, snapshot.Config.Files)

	resolvedReq := *req
	resolvedReq.Messages = append([]modality.ChatMessage(nil), req.Messages...)
	for messageIndex := range resolvedReq.Messages {
		message := &resolvedReq.Messages[messageIndex]
		if len(message.Content.Parts) == 0 {
			continue
		}
		parts := append([]modality.ContentPart(nil), message.Content.Parts...)
		for partIndex, part := range parts {
			filePart := part.File
			semanticType := strings.TrimSpace(part.Type)
			if semanticType == "document" {
				filePart = part.Document
			}
			if semanticType != "file" && semanticType != "document" {
				continue
			}
			source := commonfiles.SourceFromFilePart(filePart)
			if source.Kind == "" {
				return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "messages.content.file", "File content must include file_id, url, or data.")
			}
			info, err := h.fileUnderstandingSourceInfo(c, snapshot.Config.Files, source, projectID)
			if err != nil {
				return nil, err
			}
			if understandingPolicy.Enabled && understandingPolicy.Mode == fileUnderstandingModeDerived {
				translated, usage, err := h.deriveFileUnderstandingPart(c, snapshot.Config.Files, understandingPolicy, info)
				if err != nil {
					return nil, err
				}
				parts[partIndex] = translated
				mergeFileUnderstandingUsage(&resolvedReq, usage)
				continue
			}
			if !modelSupportsNativeFilePart(model, info.MimeType, source.Kind) {
				if !understandingPolicy.Enabled {
					return nil, fileUnderstandingRequiredError(info.MimeType)
				}
				translated, usage, err := h.deriveFileUnderstandingPart(c, snapshot.Config.Files, understandingPolicy, info)
				if err != nil {
					return nil, err
				}
				parts[partIndex] = translated
				mergeFileUnderstandingUsage(&resolvedReq, usage)
				continue
			}
			resolved, err := resolver.Resolve(c.Request.Context(), source, projectID, model.Provider)
			if err != nil {
				return nil, err
			}
			if filePart != nil {
				resolved.Citations = filePart.Citations
			}
			translated, err := resolvedFilePart(model.Provider, semanticType, resolved)
			if err != nil {
				return nil, err
			}
			parts[partIndex] = translated
		}
		message.Content.Parts = parts
	}
	return &resolvedReq, nil
}

func requestHasFileParts(req *modality.ChatRequest) bool {
	if req == nil {
		return false
	}
	for _, message := range req.Messages {
		for _, part := range message.Content.Parts {
			switch part.Type {
			case "file", "document":
				return true
			}
		}
	}
	return false
}

func resolvedFilePart(providerName string, semanticType string, resolved *commonfiles.ResolvedFile) (modality.ContentPart, error) {
	if resolved == nil {
		return modality.ContentPart{}, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "messages.content.file", "File resolution failed.")
	}
	filePart := &modality.FilePart{
		Filename:  resolved.Filename,
		MimeType:  resolved.MimeType,
		Citations: resolved.Citations,
	}
	if semanticType == "" {
		semanticType = "file"
	}

	switch resolved.Kind {
	case commonfiles.ResolvedProviderRef:
		filePart.FileID = resolved.ProviderID
		return modality.ContentPart{Type: semanticType, File: filePart, Document: documentAlias(semanticType, filePart)}, nil
	case commonfiles.ResolvedInline:
		if strings.HasPrefix(resolved.MimeType, "image/") && providerName != "anthropic" && providerName != "google" {
			return modality.ContentPart{
				Type:     "image_url",
				ImageURL: &modality.ImageURLPart{URL: commonfiles.DataURI(resolved.MimeType, resolved.B64)},
			}, nil
		}
		filePart.Data = resolved.B64
		return modality.ContentPart{Type: semanticType, File: filePart, Document: documentAlias(semanticType, filePart)}, nil
	case commonfiles.ResolvedURL:
		if strings.HasPrefix(resolved.MimeType, "image/") && providerName != "anthropic" && providerName != "google" {
			return modality.ContentPart{
				Type:     "image_url",
				ImageURL: &modality.ImageURLPart{URL: resolved.URL},
			}, nil
		}
		filePart.URL = resolved.URL
		return modality.ContentPart{Type: semanticType, File: filePart, Document: documentAlias(semanticType, filePart)}, nil
	default:
		return modality.ContentPart{}, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "provider_lacks_files_api", "messages.content.file", "Selected provider cannot accept this file reference.")
	}
}

func documentAlias(semanticType string, part *modality.FilePart) *modality.FilePart {
	if semanticType == "document" {
		return part
	}
	return nil
}

func chatProjectID(auth middleware.AuthContext) string {
	if strings.TrimSpace(auth.ProjectID) != "" {
		return strings.TrimSpace(auth.ProjectID)
	}
	if strings.TrimSpace(auth.OwnerID) != "" {
		return strings.TrimSpace(auth.OwnerID)
	}
	return "legacy-default"
}

func fileUnderstandingPolicy(req *modality.ChatRequest, filesCfg config.FilesConfig) chatFileUnderstandingPolicy {
	policy := chatFileUnderstandingPolicy{
		Config:  filesCfg.Understanding,
		Mode:    strings.TrimSpace(filesCfg.Understanding.Mode),
		Profile: understanding.NormalizeProfile(filesCfg.Understanding.Profile),
	}
	if policy.Mode == "" {
		if filesCfg.Understanding.Enabled {
			policy.Mode = "explicit"
		} else {
			policy.Mode = "disabled"
		}
	}
	if !filesCfg.Understanding.Enabled {
		return policy
	}
	if policy.Mode == "disabled" {
		return policy
	}
	options := (*modality.PolarisFileUnderstandingOptions)(nil)
	if req != nil && req.Polaris != nil {
		options = req.Polaris.FileUnderstanding
	}
	if options != nil {
		if strings.TrimSpace(options.Profile) != "" {
			policy.Profile = understanding.NormalizeProfile(options.Profile)
		}
		if options.Enabled != nil && !*options.Enabled {
			policy.Mode = "disabled"
			return policy
		}
		switch strings.TrimSpace(options.Mode) {
		case "disabled":
			policy.Mode = "disabled"
			return policy
		case fileUnderstandingModeDerived:
			policy.Enabled = true
			policy.Mode = fileUnderstandingModeDerived
			return policy
		case "auto_fallback":
			policy.Enabled = true
			policy.Mode = "auto_fallback"
			return policy
		}
		if options.Enabled != nil && *options.Enabled {
			policy.Enabled = true
			policy.Mode = fileUnderstandingModeDerived
			return policy
		}
	}
	if policy.Mode == "auto_fallback" {
		policy.Enabled = true
	}
	return policy
}

func (h *ChatHandler) fileUnderstandingSourceInfo(c *gin.Context, filesCfg config.FilesConfig, source modality.FileSource, projectID string) (fileUnderstandingInput, error) {
	info := fileUnderstandingInput{
		FileID:    firstNonEmpty(source.PolarisID, source.ProviderID),
		URL:       source.URL,
		Filename:  source.Filename,
		MimeType:  understanding.CanonicalMIME(source.MimeType, source.InlineBytes),
		ProjectID: projectID,
	}
	switch source.Kind {
	case modality.FileSourcePolarisRef:
		file, err := h.store.GetFileForProject(c.Request.Context(), source.PolarisID, projectID)
		if err != nil {
			return info, chatFileStoreError(err)
		}
		if expired(file) {
			return info, httputil.NewError(http.StatusGone, "invalid_request_error", "file_expired", "file_id", "File has expired.")
		}
		info.FileID = file.PolarisID
		info.Sha256 = file.Sha256
		info.Filename = file.OriginalFilename
		info.MimeType = understanding.CanonicalMIME(file.MimeType, nil)
		info.Size = file.Size
		info.Source = "polaris_ref"
		return info, nil
	case modality.FileSourceInline:
		data := append([]byte(nil), source.InlineBytes...)
		if len(data) == 0 && strings.TrimSpace(source.InlineB64) != "" {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(source.InlineB64))
			if err != nil {
				return info, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "messages.content.file", "Inline file data must be base64 encoded.")
			}
			data = decoded
		}
		sha := sha256.Sum256(data)
		info.Data = data
		info.Sha256 = hex.EncodeToString(sha[:])
		info.Size = int64(len(data))
		info.MimeType = understanding.CanonicalMIME(source.MimeType, data)
		info.Source = "inline"
		return info, nil
	case modality.FileSourceURL:
		info.MimeType = understanding.CanonicalMIME(source.MimeType, nil)
		info.Source = "url"
		return info, nil
	case modality.FileSourceProviderRef:
		info.Source = "provider_ref"
		return info, nil
	default:
		return info, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file_source", "messages.content.file", "Unsupported file source.")
	}
}

func modelSupportsNativeFilePart(model provider.Model, mimeType string, sourceKind modality.FileSourceKind) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return modelHasCapability(model, modality.CapabilityVision)
	case strings.HasPrefix(mimeType, "audio/"):
		return modelHasCapability(model, modality.CapabilityAudioInput)
	case mimeType == "application/pdf":
		return modelHasCapability(model, modality.CapabilityPDFInput) || modelHasCapability(model, modality.CapabilityPDF)
	case mimeType == "" || sourceKind == modality.FileSourceProviderRef:
		return modelHasCapability(model, modality.CapabilityFileReference)
	default:
		return modelHasCapability(model, modality.CapabilityDocumentInput) || modelHasCapability(model, modality.CapabilityFileReference)
	}
}

func modelHasCapability(model provider.Model, capability modality.Capability) bool {
	for _, existing := range model.Capabilities {
		if existing == capability {
			return true
		}
	}
	return false
}

func fileUnderstandingRequiredError(mimeType string) error {
	mimeType = strings.TrimSpace(mimeType)
	message := "Selected model does not support the required native file or image capability."
	if mimeType != "" {
		message = fmt.Sprintf("Selected model does not support native %s input.", mimeType)
	}
	return httputil.NewError(http.StatusBadRequest, "capability_not_supported", "capability_missing", "messages.content.file", message+" Polaris beta file understanding is disabled by default; enable files.understanding and opt in per request to use derived text fallback.")
}

func (h *ChatHandler) deriveFileUnderstandingPart(c *gin.Context, filesCfg config.FilesConfig, policy chatFileUnderstandingPolicy, info fileUnderstandingInput) (modality.ContentPart, *modality.FileUnderstandingUsage, error) {
	if info.Source == "provider_ref" {
		return modality.ContentPart{}, nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "provider_ref_not_understandable", "messages.content.file", "Provider-native file ids cannot be converted by Polaris because Polaris does not own the bytes.")
	}
	input, err := h.loadFileUnderstandingInput(c, filesCfg, info)
	if err != nil {
		return modality.ContentPart{}, nil, err
	}
	maxBytes := effectiveUnderstandingMaxBytes(policy.Config.MaxBytes)
	if maxBytes > 0 && input.Size > maxBytes {
		return modality.ContentPart{}, nil, httputil.NewError(http.StatusRequestEntityTooLarge, "invalid_request_error", "file_too_large", "messages.content.file", "File exceeds files.understanding.max_bytes for beta derived-context fallback.")
	}
	input.MimeType = understanding.CanonicalMIME(input.MimeType, input.Data)
	registry, err := fileUnderstandingRegistry(policy.Config)
	if err != nil {
		return modality.ContentPart{}, nil, httputil.NewError(http.StatusInternalServerError, "internal_error", "file_understanding_config_invalid", "files.understanding.processors", "Configured Polaris file understanding processor is invalid.")
	}
	planOptions := understanding.DefaultPlanOptionsForDerivedContext(input.MimeType, policy.Profile)
	plan, err := registry.Plan(understanding.Input{MimeType: input.MimeType, Data: input.Data}, planOptions)
	if err != nil {
		return modality.ContentPart{}, nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "file_understanding_processor_missing", "messages.content.file", "No Polaris beta file understanding processor supports this MIME type.")
	}
	processor := plan.PrimaryProcessor()
	if processor == nil {
		return modality.ContentPart{}, nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "file_understanding_processor_missing", "messages.content.file", "No Polaris beta file understanding processor supports this MIME type.")
	}

	artifact, artifactSource, err := h.fileUnderstandingArtifact(c.Request.Context(), filesCfg, policy, input, processor)
	if err != nil {
		return modality.ContentPart{}, nil, err
	}
	artifacts := []understanding.Artifact{*artifact}
	refs := []modality.FileUnderstandingArtifactRef{fileUnderstandingArtifactRef(input, artifact, artifactSource)}
	if policy.Config.Chunking.Enabled && strings.TrimSpace(artifact.Text) != "" {
		chunkArtifact, err := understanding.ProcessWithProcessor(c.Request.Context(), understanding.ChunkProcessor{
			MaxChars:     policy.Config.Chunking.MaxChars,
			OverlapChars: policy.Config.Chunking.OverlapChars,
		}, understanding.Input{
			FileID:      input.FileID,
			Sha256:      input.Sha256,
			Filename:    input.Filename,
			MimeType:    input.MimeType,
			Size:        input.Size,
			DerivedText: artifact.Text,
			Artifacts:   artifacts,
		})
		if err == nil {
			artifacts = append(artifacts, *chunkArtifact)
			refs = append(refs, fileUnderstandingArtifactRef(input, chunkArtifact, "generated"))
		}
	}
	warning := artifact.Warning
	if len(artifacts) > 1 && artifacts[1].Warning != "" {
		warning = strings.TrimSpace(warning + " " + artifacts[1].Warning)
	}
	usage := &modality.FileUnderstandingUsage{
		Used:      true,
		Mode:      firstNonEmpty(policy.Mode, fileUnderstandingModeDerived),
		Profile:   string(policy.Profile),
		Warning:   warning,
		Artifacts: refs,
	}
	c.Header("X-Polaris-File-Understanding", usage.Mode)
	return modality.ContentPart{Type: "text", Text: formatFileUnderstandingContext(input, artifacts, usage.Mode)}, usage, nil
}

func fileUnderstandingArtifactRef(input fileUnderstandingInput, artifact *understanding.Artifact, source string) modality.FileUnderstandingArtifactRef {
	if artifact == nil {
		return modality.FileUnderstandingArtifactRef{}
	}
	return modality.FileUnderstandingArtifactRef{
		FileID:    input.FileID,
		Sha256:    input.Sha256,
		MimeType:  input.MimeType,
		Filename:  input.Filename,
		Kind:      string(artifact.Kind),
		Processor: artifact.Processor,
		Version:   artifact.Version,
		Source:    source,
	}
}

func firstArtifactKind(kind understanding.ArtifactKind, metadataJSON string) understanding.ArtifactKind {
	if kind != "" {
		return kind
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return understanding.ArtifactText
	}
	if value, ok := metadata["artifact_kind"].(string); ok && strings.TrimSpace(value) != "" {
		return understanding.ArtifactKind(strings.TrimSpace(value))
	}
	return understanding.ArtifactText
}

func (h *ChatHandler) loadFileUnderstandingInput(c *gin.Context, filesCfg config.FilesConfig, info fileUnderstandingInput) (fileUnderstandingInput, error) {
	switch info.Source {
	case "polaris_ref":
		file, err := h.store.GetFileForProject(c.Request.Context(), info.FileID, info.ProjectID)
		if err != nil {
			return info, chatFileStoreError(err)
		}
		if expired(file) {
			return info, httputil.NewError(http.StatusGone, "invalid_request_error", "file_expired", "file_id", "File has expired.")
		}
		resolver := commonfiles.Resolver{Store: h.store, Config: filesCfg}
		data, err := resolver.ReadFileBytes(c.Request.Context(), file)
		if err != nil {
			return info, err
		}
		info.Data = data
		info.FileID = file.PolarisID
		info.Sha256 = file.Sha256
		info.Filename = file.OriginalFilename
		info.MimeType = file.MimeType
		info.Size = file.Size
		return info, nil
	case "inline":
		if len(info.Data) == 0 {
			return info, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "messages.content.file", "Inline file data is empty.")
		}
		return info, nil
	case "url":
		client := transport.NewSSRFClient(filesCfg.SSRF, cfgTimeout(c))
		resp, err := client.Get(c.Request.Context(), info.URL)
		if err != nil {
			return info, ssrfError(err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return info, httputil.NewError(http.StatusBadGateway, "provider_error", "origin_fetch_failed", "url", "File URL did not return a successful response.")
		}
		data, err := readLimitedUnderstandingBytes(resp.Body, effectiveUnderstandingMaxBytes(filesCfg.Understanding.MaxBytes))
		if err != nil {
			return info, err
		}
		sha := sha256.Sum256(data)
		info.Data = data
		info.Sha256 = hex.EncodeToString(sha[:])
		info.Size = int64(len(data))
		info.MimeType = understanding.CanonicalMIME(firstNonEmpty(info.MimeType, resp.Header.Get("Content-Type")), data)
		return info, nil
	default:
		return info, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "file_understanding_source_unsupported", "messages.content.file", "Polaris beta file understanding can only process Polaris, inline, or URL file sources.")
	}
}

func (h *ChatHandler) fileUnderstandingArtifact(ctx context.Context, filesCfg config.FilesConfig, policy chatFileUnderstandingPolicy, input fileUnderstandingInput, processor understanding.Processor) (*understanding.Artifact, string, error) {
	cacheEnabled := filesCfg.Understanding.CacheArtifacts && h.store != nil && input.Sha256 != ""
	if cacheEnabled {
		if cached, ok, err := h.store.GetFileUnderstandingArtifact(ctx, input.Sha256, processor.Name(), processor.Version()); err != nil {
			return nil, "", err
		} else if ok {
			artifact, decoded := understanding.ArtifactFromJSON(cached.ArtifactJSON)
			if !decoded {
				artifact = &understanding.Artifact{}
			}
			artifact.Kind = firstArtifactKind(artifact.Kind, cached.MetadataJSON)
			artifact.Processor = firstNonEmpty(artifact.Processor, cached.Processor)
			artifact.Version = firstNonEmpty(artifact.Version, cached.Version)
			artifact.MimeType = firstNonEmpty(input.MimeType, cached.MimeType)
			artifact.Text = firstNonEmpty(artifact.Text, cached.Text)
			artifact.Warning = firstNonEmpty(artifact.Warning, cached.Warning)
			understanding.RebindArtifactProvenance(artifact, understanding.Input{
				FileID:   input.FileID,
				Sha256:   input.Sha256,
				Filename: input.Filename,
				MimeType: input.MimeType,
			})
			return artifact, "cache", nil
		}
	}
	artifact, err := understanding.ProcessWithProcessor(ctx, processor, understanding.Input{
		FileID:       input.FileID,
		Sha256:       input.Sha256,
		Filename:     input.Filename,
		MimeType:     input.MimeType,
		Size:         input.Size,
		Data:         input.Data,
		MaxTextChars: policy.Config.MaxTextChars,
	})
	if err != nil {
		var unsupported understanding.ErrUnsupportedMIME
		if errors.As(err, &unsupported) {
			return nil, "", httputil.NewError(http.StatusBadRequest, "capability_not_supported", "file_understanding_processor_missing", "messages.content.file", "No Polaris beta file understanding processor supports this MIME type.")
		}
		return nil, "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "file_understanding_failed", "messages.content.file", "Polaris beta file understanding failed to process the file.")
	}
	if cacheEnabled {
		if err := h.store.PutFileUnderstandingArtifact(ctx, store.FileUnderstandingArtifact{
			Sha256:       input.Sha256,
			Processor:    artifact.Processor,
			Version:      artifact.Version,
			MimeType:     artifact.MimeType,
			Text:         artifact.Text,
			Warning:      artifact.Warning,
			MetadataJSON: understanding.MetadataJSON(artifact.Metadata),
			ArtifactJSON: understanding.CacheableArtifactJSON(artifact),
			CreatedAt:    time.Now().UTC(),
		}); err != nil {
			return nil, "", err
		}
	}
	return artifact, "generated", nil
}

func formatFileUnderstandingContext(input fileUnderstandingInput, artifacts []understanding.Artifact, mode string) string {
	if len(artifacts) == 0 {
		return ""
	}
	artifact := artifacts[0]
	var b strings.Builder
	b.WriteString("[Polaris beta file understanding: ")
	b.WriteString(firstNonEmpty(mode, fileUnderstandingModeDerived))
	b.WriteString("]\n")
	if artifact.Warning != "" {
		b.WriteString("Warning: ")
		b.WriteString(artifact.Warning)
		b.WriteString("\n")
	}
	if input.FileID != "" {
		b.WriteString("File ID: ")
		b.WriteString(input.FileID)
		b.WriteString("\n")
	}
	if input.Filename != "" {
		b.WriteString("Filename: ")
		b.WriteString(understanding.HumanFilename(input.Filename))
		b.WriteString("\n")
	}
	if input.Sha256 != "" {
		b.WriteString("SHA-256: ")
		b.WriteString(input.Sha256)
		b.WriteString("\n")
	}
	if input.MimeType != "" {
		b.WriteString("MIME type: ")
		b.WriteString(input.MimeType)
		b.WriteString("\n")
	}
	b.WriteString("Processor: ")
	b.WriteString(artifact.Processor)
	b.WriteString(" ")
	b.WriteString(artifact.Version)
	if artifact.Kind != "" {
		b.WriteString(" (")
		b.WriteString(string(artifact.Kind))
		b.WriteString(")")
	}
	if len(artifacts) > 1 {
		b.WriteString("\nArtifacts:")
		for _, current := range artifacts {
			b.WriteString("\n- ")
			b.WriteString(current.Processor)
			b.WriteString(" ")
			b.WriteString(current.Version)
			if current.Kind != "" {
				b.WriteString(" ")
				b.WriteString(string(current.Kind))
			}
			if len(current.Chunks) > 0 {
				b.WriteString(" chunks=")
				fmt.Fprintf(&b, "%d", len(current.Chunks))
			}
		}
	}
	b.WriteString("\n\nDerived context:\n")
	b.WriteString(artifact.Text)
	return b.String()
}

func mergeFileUnderstandingUsage(req *modality.ChatRequest, usage *modality.FileUnderstandingUsage) {
	if req == nil || usage == nil {
		return
	}
	if req.FileUnderstandingUsage == nil {
		copied := *usage
		copied.Artifacts = append([]modality.FileUnderstandingArtifactRef(nil), usage.Artifacts...)
		req.FileUnderstandingUsage = &copied
		return
	}
	req.FileUnderstandingUsage.Used = req.FileUnderstandingUsage.Used || usage.Used
	req.FileUnderstandingUsage.Mode = firstNonEmpty(req.FileUnderstandingUsage.Mode, usage.Mode)
	req.FileUnderstandingUsage.Profile = firstNonEmpty(req.FileUnderstandingUsage.Profile, usage.Profile)
	req.FileUnderstandingUsage.Warning = firstNonEmpty(req.FileUnderstandingUsage.Warning, usage.Warning)
	req.FileUnderstandingUsage.Artifacts = append(req.FileUnderstandingUsage.Artifacts, usage.Artifacts...)
}

func attachFileUnderstandingMetadata(response *modality.ChatResponse, req *modality.ChatRequest) {
	if response == nil || req == nil || req.FileUnderstandingUsage == nil {
		return
	}
	if response.Polaris == nil {
		response.Polaris = &modality.ChatPolarisMetadata{}
	}
	response.Polaris.FileUnderstanding = req.FileUnderstandingUsage
}

func effectiveUnderstandingMaxBytes(value int64) int64 {
	if value <= 0 {
		return config.DefaultInlineFileBytes
	}
	return value
}

func readLimitedUnderstandingBytes(body io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = config.DefaultInlineFileBytes
	}
	var buf bytes.Buffer
	buf.Grow(int(minInt64(maxBytes, 64*1024)))
	n, err := io.Copy(&buf, io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if n > maxBytes {
		return nil, httputil.NewError(http.StatusRequestEntityTooLarge, "invalid_request_error", "file_too_large", "messages.content.file", "File exceeds files.understanding.max_bytes for beta derived-context fallback.")
	}
	return buf.Bytes(), nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func chatFileStoreError(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "file_id", "File was not found.")
	}
	return err
}
