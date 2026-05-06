package e2e

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/pkg/client"
)

const (
	evalTextCode       = "TXT_EVAL_4812"
	evalPDFCode        = "PDF_EVAL_5823"
	evalDOCXCode       = "DOCX_EVAL_6934"
	evalImageCode      = "IMG_EVAL_7145"
	evalNativeImageKey = "RED_NATIVE_OK"
)

type fileUnderstandingEvalResult struct {
	Model        string   `json:"model"`
	Resolved     string   `json:"resolved"`
	Provider     string   `json:"provider"`
	Case         string   `json:"case"`
	Mode         string   `json:"mode"`
	MimeType     string   `json:"mime_type"`
	Expected     string   `json:"expected"`
	Answer       string   `json:"answer"`
	Passed       bool     `json:"passed"`
	LatencyMS    int64    `json:"latency_ms"`
	PromptTokens int      `json:"prompt_tokens,omitempty"`
	OutputTokens int      `json:"output_tokens,omitempty"`
	TotalTokens  int      `json:"total_tokens,omitempty"`
	UsageSource  string   `json:"usage_source,omitempty"`
	Artifacts    []string `json:"artifacts,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func TestLiveFileUnderstandingEval(t *testing.T) {
	if os.Getenv("POLARIS_FILE_UNDERSTANDING_EVAL") != "1" {
		t.Skip("POLARIS_FILE_UNDERSTANDING_EVAL is not set")
	}

	processor := newEvalRemoteProcessor(t)
	harness := newLiveSmokeHarnessWithOptions(t, liveSmokeHarnessOptions{
		configure: func(cfg *config.Config) {
			cfg.Files.Enabled = true
			cfg.Files.Understanding.Enabled = true
			cfg.Files.Understanding.Mode = "auto_fallback"
			cfg.Files.Understanding.Profile = "quality"
			cfg.Files.Understanding.MaxBytes = 5 * 1024 * 1024
			cfg.Files.Understanding.MaxTextChars = 16000
			cfg.Files.Understanding.CacheArtifacts = true
			cfg.Files.Understanding.Chunking.Enabled = true
			cfg.Files.Understanding.Chunking.MaxChars = 900
			cfg.Files.Understanding.Chunking.OverlapChars = 120
			cfg.Files.Understanding.Processors = []config.FileUnderstandingProcessorConfig{
				{
					Enabled:      true,
					Name:         "eval_remote_captioner",
					Backend:      "remote_http",
					Endpoint:     processor.URL,
					Priority:     900,
					MIMETypes:    []string{"image/*"},
					Artifacts:    []string{"image_caption", "ocr_text"},
					Capabilities: []string{"image_caption", "ocr"},
					Profiles:     []string{"quality"},
				},
			}
		},
	})

	ctx := context.Background()
	models := evalModelMatrix(t, harness)
	if len(models) == 0 {
		t.Skip("no file understanding eval models are configured")
	}

	fixtures := []struct {
		name     string
		filename string
		mime     string
		data     []byte
		expected string
	}{
		{name: "derived_text_txt", filename: "eval.txt", mime: "text/plain", data: []byte("The answer code is " + evalTextCode + "."), expected: evalTextCode},
		{name: "derived_text_pdf", filename: "eval.pdf", mime: "application/pdf", data: evalPDFBytes(t, "The answer code is "+evalPDFCode+"."), expected: evalPDFCode},
		{name: "derived_text_docx", filename: "eval.docx", mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", data: evalDOCXBytes(t, "The answer code is "+evalDOCXCode+"."), expected: evalDOCXCode},
		{name: "derived_image_remote_caption", filename: "eval.png", mime: "image/png", data: evalSolidPNG(t, color.RGBA{R: 220, A: 255}), expected: evalImageCode},
	}

	uploads := map[string]*client.FileObject{}
	for _, fixture := range fixtures {
		uploaded, err := harness.client.UploadFile(ctx, &client.FileUploadRequest{
			File:        fixture.data,
			Filename:    fixture.filename,
			ContentType: fixture.mime,
			Purpose:     "user_data",
			MimeType:    fixture.mime,
			Metadata:    map[string]string{"eval_case": fixture.name},
		})
		if err != nil {
			t.Fatalf("upload %s: %v", fixture.name, err)
		}
		uploads[fixture.name] = uploaded
	}

	var results []fileUnderstandingEvalResult
	for _, modelName := range models {
		modelName := modelName
		t.Run(strings.ReplaceAll(modelName, "/", "_"), func(t *testing.T) {
			model, err := harness.registry.ResolveModel(modelName)
			if err != nil {
				t.Fatalf("resolve eval model %s: %v", modelName, err)
			}
			policy := harness.gateCase(t, "file understanding eval "+modelName, modelName)
			harness.requireEnv(t, policy, "file understanding eval "+modelName, requiredProviderEnv(model.Provider)...)

			for _, fixture := range fixtures {
				result := runFileUnderstandingEvalCase(t, ctx, harness, modelName, model, fixture.name, "derived_context", uploads[fixture.name], fixture.mime, fixture.expected)
				results = append(results, result)
				if !result.Passed {
					t.Errorf("eval failed: case=%s mode=%s answer=%q error=%q", result.Case, result.Mode, result.Answer, result.Error)
				}
			}

			if modelHasEvalCapability(model, modality.CapabilityVision) {
				result := runFileUnderstandingEvalCase(t, ctx, harness, modelName, model, "native_image_file", "native", uploads["derived_image_remote_caption"], "image/png", evalNativeImageKey)
				results = append(results, result)
				if !result.Passed {
					t.Errorf("eval failed: case=%s mode=%s answer=%q error=%q", result.Case, result.Mode, result.Answer, result.Error)
				}
			}
			if modelHasEvalCapability(model, modality.CapabilityPDFInput) || modelHasEvalCapability(model, modality.CapabilityPDF) {
				result := runFileUnderstandingEvalCase(t, ctx, harness, modelName, model, "native_pdf_file", "native", uploads["derived_text_pdf"], "application/pdf", evalPDFCode)
				results = append(results, result)
				if !result.Passed {
					t.Errorf("eval failed: case=%s mode=%s answer=%q error=%q", result.Case, result.Mode, result.Answer, result.Error)
				}
			}
		})
	}

	writeEvalReport(t, results)
	if len(results) == 0 {
		t.Skip("all file understanding eval models were skipped")
	}
	var failed int
	for _, result := range results {
		if !result.Passed {
			failed++
			continue
		}
		t.Logf("file_understanding_eval model=%s case=%s mode=%s latency_ms=%d tokens=%d artifacts=%s answer=%q",
			result.Model, result.Case, result.Mode, result.LatencyMS, result.TotalTokens, strings.Join(result.Artifacts, ","), result.Answer)
	}
	if failed > 0 {
		t.Fatalf("file understanding eval failed %d/%d cases; report contains details", failed, len(results))
	}
}

func runFileUnderstandingEvalCase(t *testing.T, ctx context.Context, harness *liveSmokeHarness, modelName string, model provider.Model, caseName string, mode string, file *client.FileObject, mimeType string, expected string) fileUnderstandingEvalResult {
	t.Helper()
	start := time.Now()
	result := fileUnderstandingEvalResult{
		Model:     modelName,
		Resolved:  model.ID,
		Provider:  model.Provider,
		Case:      caseName,
		Mode:      mode,
		MimeType:  mimeType,
		Expected:  expected,
		LatencyMS: 0,
	}

	temperature := 0.0
	prompt := fmt.Sprintf("Use the attached file. Reply with only %s and no punctuation.", expected)
	if caseName == "native_image_file" {
		prompt = "Inspect the attached image. If the image is primarily red, reply with RED_NATIVE_OK only. Otherwise reply FAIL."
	}
	req := &client.ChatCompletionRequest{
		Model:       modelName,
		Temperature: &temperature,
		MaxTokens:   evalMaxTokens(model),
		Messages: []client.ChatMessage{{
			Role: "user",
			Content: client.NewPartContent(
				client.ContentPart{Type: "text", Text: prompt},
				client.ContentPart{Type: "file", File: &client.FilePart{
					FileID:   file.ID,
					MimeType: mimeType,
					Filename: file.Filename,
				}},
			),
		}},
	}
	if mode == "derived_context" {
		enabled := true
		req.Polaris = &client.PolarisChatOptions{
			FileUnderstanding: &client.PolarisFileUnderstandingOptions{
				Enabled: &enabled,
				Mode:    "derived_context",
				Profile: "quality",
			},
		}
	}

	resp, err := harness.client.CreateChatCompletion(ctx, req)
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if len(resp.Choices) > 0 && resp.Choices[0].Message.Content.Text != nil {
		result.Answer = strings.TrimSpace(*resp.Choices[0].Message.Content.Text)
	}
	result.PromptTokens = resp.Usage.PromptTokens
	result.OutputTokens = resp.Usage.CompletionTokens
	result.TotalTokens = resp.Usage.TotalTokens
	result.UsageSource = resp.Usage.Source
	if resp.Polaris != nil && resp.Polaris.FileUnderstanding != nil {
		for _, artifact := range resp.Polaris.FileUnderstanding.Artifacts {
			label := strings.TrimSpace(artifact.Kind + ":" + artifact.Processor + ":" + artifact.Source)
			result.Artifacts = append(result.Artifacts, label)
		}
	}
	result.Passed = strings.Contains(result.Answer, expected)
	return result
}

func evalMaxTokens(model provider.Model) int {
	if modelHasEvalCapability(model, modality.CapabilityReasoning) {
		return 256
	}
	return 64
}

func evalModelMatrix(t *testing.T, harness *liveSmokeHarness) []string {
	t.Helper()
	if raw := strings.TrimSpace(os.Getenv("POLARIS_FILE_UNDERSTANDING_EVAL_MODELS")); raw != "" {
		return splitCSV(raw)
	}
	if os.Getenv("POLARIS_FILE_UNDERSTANDING_EVAL_ALL") == "1" {
		return allConfiguredChatModels(harness)
	}
	return availableEvalModels(t, harness, []string{
		"openai-mini-chat",
		"anthropic-haiku-chat",
		"google-flash-lite-chat",
		"xai-fast-chat",
		"deepseek-flash-chat",
		"deepseek-chat",
		"bytedance-chat",
		"bytedance-vision",
		"qwen-flash-chat",
	})
}

func availableEvalModels(t *testing.T, harness *liveSmokeHarness, candidates []string) []string {
	t.Helper()
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, err := harness.registry.ResolveModel(candidate); err != nil {
			t.Logf("skipping unavailable file understanding eval model %s: %v", candidate, err)
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func allConfiguredChatModels(harness *liveSmokeHarness) []string {
	models := harness.registry.ListModels(true)
	seen := map[string]struct{}{}
	var out []string
	for _, model := range models {
		if model.Modality != modality.ModalityChat {
			continue
		}
		if model.Kind != "" {
			continue
		}
		name := model.ID
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func requiredProviderEnv(providerName string) []string {
	switch providerName {
	case "openai":
		return []string{"OPENAI_API_KEY"}
	case "anthropic":
		return []string{"ANTHROPIC_API_KEY"}
	case "google":
		return []string{"GOOGLE_API_KEY"}
	case "xai":
		return []string{"XAI_API_KEY"}
	case "deepseek":
		return []string{"DEEPSEEK_API_KEY"}
	case "bytedance":
		return []string{"VOLCENGINE_ARK_API_KEY"}
	case "qwen":
		return []string{"DASHSCOPE_API_KEY"}
	case "groq":
		return []string{"GROQ_API_KEY"}
	case "mistral":
		return []string{"MISTRAL_API_KEY"}
	case "moonshot":
		return []string{"MOONSHOT_API_KEY"}
	case "glm":
		return []string{"GLM_API_KEY"}
	case "openrouter":
		return []string{"OPENROUTER_API_KEY"}
	case "together":
		return []string{"TOGETHER_API_KEY"}
	case "fireworks":
		return []string{"FIREWORKS_API_KEY"}
	case "featherless":
		return []string{"FEATHERLESS_API_KEY"}
	case "nvidia":
		return []string{"NVIDIA_API_KEY"}
	case "bedrock":
		return []string{"AWS_BEDROCK_ACCESS_KEY_ID", "AWS_BEDROCK_SECRET_ACCESS_KEY", "AWS_BEDROCK_REGION"}
	default:
		return nil
	}
}

func modelHasEvalCapability(model provider.Model, capability modality.Capability) bool {
	for _, existing := range model.Capabilities {
		if existing == capability {
			return true
		}
	}
	return false
}

func newEvalRemoteProcessor(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			FileID   string `json:"file_id"`
			MimeType string `json:"mime_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode processor request: %v", err)
		}
		if !strings.HasPrefix(req.MimeType, "image/") {
			t.Fatalf("unexpected processor MIME %q", req.MimeType)
		}
		resp := map[string]any{
			"artifact": map[string]any{
				"kind": "image_caption",
				"text": "Remote image caption artifact. The required answer code is " + evalImageCode + ". The image is a red square.",
				"metadata": map[string]any{
					"artifact_kind":     "image_caption",
					"source_preserving": false,
				},
				"provenance": []map[string]any{
					{
						"file_id":       req.FileID,
						"artifact_kind": "image_caption",
						"start_char":    0,
						"end_char":      102,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func writeEvalReport(t *testing.T, results []fileUnderstandingEvalResult) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("POLARIS_FILE_UNDERSTANDING_EVAL_REPORT"))
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create eval report dir: %v", err)
	}
	payload, err := json.MarshalIndent(struct {
		GeneratedAt time.Time                     `json:"generated_at"`
		Results     []fileUnderstandingEvalResult `json:"results"`
	}{GeneratedAt: time.Now().UTC(), Results: results}, "", "  ")
	if err != nil {
		t.Fatalf("marshal eval report: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write eval report: %v", err)
	}
	t.Logf("file understanding eval report written to %s", path)
}

func evalPDFBytes(t *testing.T, text string) []byte {
	t.Helper()
	content := "BT /F1 12 Tf 72 720 Td (" + escapePDFLiteral(text) + ") Tj ET\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content),
	}
	var pdf bytes.Buffer
	offsets := make([]int, 0, len(objects))
	_, _ = pdf.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	for i, object := range objects {
		offsets = append(offsets, pdf.Len())
		_, _ = fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xrefOffset := pdf.Len()
	_, _ = fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	_, _ = pdf.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		_, _ = fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	_, _ = fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return pdf.Bytes()
}

func escapePDFLiteral(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, "(", `\(`)
	text = strings.ReplaceAll(text, ")", `\)`)
	return text
}

func evalDOCXBytes(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":          `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`,
		"word/document.xml":            `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`,
		"_rels/.rels":                  `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
	}
	for name, body := range files {
		part, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := part.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func evalSolidPNG(t *testing.T, fill color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
