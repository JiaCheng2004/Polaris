package understanding

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteProcessorReturnsCanonicalArtifact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var req remoteProcessorRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		decoded, err := base64.StdEncoding.DecodeString(req.DataBase64)
		if err != nil {
			t.Fatalf("decode base64: %v", err)
		}
		if string(decoded) != "image bytes" {
			t.Fatalf("unexpected data %q", decoded)
		}
		_ = json.NewEncoder(w).Encode(remoteProcessorResponse{
			Artifact: &Artifact{
				Kind: ArtifactImageCaption,
				Text: "A receipt on a desk.",
				Metadata: map[string]any{
					"artifact_kind": string(ArtifactImageCaption),
				},
			},
		})
	}))
	defer server.Close()

	processor, err := NewRemoteProcessor(RemoteProcessorConfig{
		Name:          "captioner",
		Endpoint:      server.URL,
		MIMETypes:     []string{"image/*"},
		ArtifactKinds: []ArtifactKind{ArtifactImageCaption},
		Capabilities:  []ProcessorCapability{CapabilityImageCaption},
		Profiles:      []ProcessingProfile{ProfileQuality},
		Priority:      500,
	})
	if err != nil {
		t.Fatalf("NewRemoteProcessor() error = %v", err)
	}
	artifact, err := ProcessWithProcessor(context.Background(), processor, Input{
		FileID:   "pl_file_test",
		Sha256:   "sha",
		Filename: "image.png",
		MimeType: "image/png",
		Data:     []byte("image bytes"),
	})
	if err != nil {
		t.Fatalf("ProcessWithProcessor(remote) error = %v", err)
	}
	if artifact.Kind != ArtifactImageCaption || artifact.Text != "A receipt on a desk." {
		t.Fatalf("unexpected artifact %#v", artifact)
	}
	if len(artifact.Provenance) != 1 || artifact.Provenance[0].FileID != "pl_file_test" {
		t.Fatalf("expected provenance, got %#v", artifact.Provenance)
	}
}

func TestTikaProcessorPUTsBytesAndNormalizesText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/tika" {
			t.Fatalf("request = %s %s, want PUT /tika", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/pdf" {
			t.Fatalf("content-type = %q", r.Header.Get("Content-Type"))
		}
		_, _ = w.Write([]byte("Hello from Tika\n\nTable row"))
	}))
	defer server.Close()

	processor, err := NewTikaProcessor(TikaProcessorConfig{
		Name:      "tika",
		Endpoint:  server.URL,
		MIMETypes: []string{"application/pdf"},
	})
	if err != nil {
		t.Fatalf("NewTikaProcessor() error = %v", err)
	}
	artifact, err := ProcessWithProcessor(context.Background(), processor, Input{
		MimeType: "application/pdf",
		Data:     []byte("%PDF-"),
	})
	if err != nil {
		t.Fatalf("ProcessWithProcessor(tika) error = %v", err)
	}
	if !strings.Contains(artifact.Text, "Hello from Tika") || artifact.Metadata["backend"] != "tika_server" {
		t.Fatalf("unexpected artifact %#v", artifact)
	}
}

func TestChunkProcessorPreservesSourceSpans(t *testing.T) {
	text := "alpha beta gamma delta epsilon zeta eta theta iota kappa"
	artifact, err := ProcessWithProcessor(context.Background(), ChunkProcessor{MaxChars: 20, OverlapChars: 5}, Input{
		FileID:      "pl_file_chunks",
		Sha256:      "sha",
		Filename:    "source.txt",
		MimeType:    "text/plain",
		DerivedText: text,
	})
	if err != nil {
		t.Fatalf("ProcessWithProcessor(chunk) error = %v", err)
	}
	if artifact.Kind != ArtifactChunk || len(artifact.Chunks) < 2 {
		t.Fatalf("expected chunk artifact, got %#v", artifact)
	}
	for _, chunk := range artifact.Chunks {
		if chunk.Source.FileID != "pl_file_chunks" || chunk.StartChar >= chunk.EndChar {
			t.Fatalf("invalid chunk provenance %#v", chunk)
		}
	}
}

func TestCacheableArtifactJSONScrubsAndRebindsProvenance(t *testing.T) {
	original := &Artifact{
		Kind:      ArtifactLayout,
		Processor: "layout",
		Version:   "v1",
		Text:      "hello",
		Provenance: []Provenance{{
			FileID:    "pl_file_original",
			Sha256:    "old_sha",
			Filename:  "old.pdf",
			MimeType:  "application/pdf",
			StartChar: 2,
			EndChar:   5,
		}},
		Chunks: []Chunk{{
			ID:        "chunk_0001",
			Text:      "hello",
			StartChar: 0,
			EndChar:   5,
			Source: Provenance{
				FileID:   "pl_file_original",
				Filename: "old.pdf",
			},
		}},
	}
	cached, ok := ArtifactFromJSON(CacheableArtifactJSON(original))
	if !ok {
		t.Fatal("ArtifactFromJSON(cacheable) = false")
	}
	if cached.Provenance[0].FileID != "" || cached.Chunks[0].Source.Filename != "" {
		t.Fatalf("cacheable artifact leaked source identifiers: %#v", cached)
	}
	RebindArtifactProvenance(cached, Input{
		FileID:   "pl_file_current",
		Sha256:   "current_sha",
		Filename: "current.pdf",
		MimeType: "application/pdf",
	})
	if cached.Provenance[0].FileID != "pl_file_current" || cached.Chunks[0].Source.Filename != "current.pdf" {
		t.Fatalf("artifact provenance was not rebound: %#v", cached)
	}
	if cached.Provenance[0].StartChar != 2 || cached.Provenance[0].EndChar != 5 {
		t.Fatalf("source span was not preserved: %#v", cached.Provenance[0])
	}
}

func TestPipelineSelectsQualityRemoteBeforeBuiltin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(remoteProcessorResponse{
			Artifact: &Artifact{
				Kind: ArtifactOCRText,
				Text: "quality OCR text",
				Metadata: map[string]any{
					"artifact_kind": string(ArtifactOCRText),
				},
			},
		})
	}))
	defer server.Close()

	remote, err := NewRemoteProcessor(RemoteProcessorConfig{
		Name:          "ocr_layout",
		Endpoint:      server.URL,
		MIMETypes:     []string{"application/pdf"},
		ArtifactKinds: []ArtifactKind{ArtifactOCRText, ArtifactLayout, ArtifactTable, ArtifactForm},
		Capabilities:  []ProcessorCapability{CapabilityOCR, CapabilityLayout, CapabilityTableExtract, CapabilityFormExtract},
		Profiles:      []ProcessingProfile{ProfileQuality},
		Priority:      500,
	})
	if err != nil {
		t.Fatalf("NewRemoteProcessor() error = %v", err)
	}
	registry := NewRegistryWithRemoteProcessors(remote)
	result, err := ProcessPipeline(context.Background(), registry, Input{
		MimeType: "application/pdf",
		Data:     []byte("%PDF-1.4"),
	}, PipelineOptions{
		PlanOptions: DefaultPlanOptionsForDerivedContext("application/pdf", ProfileQuality),
		Chunking:    ChunkOptions{Enabled: true, MaxChars: 100, OverlapChars: 10},
	})
	if err != nil {
		t.Fatalf("ProcessPipeline() error = %v", err)
	}
	if got := result.Artifacts[0].Processor; got != "ocr_layout" {
		t.Fatalf("primary processor = %q, want remote ocr_layout", got)
	}
	if len(result.Artifacts) != 2 || result.Artifacts[1].Kind != ArtifactChunk {
		t.Fatalf("expected chunk post-processor, got %#v", result.Artifacts)
	}
}
