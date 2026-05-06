package understanding

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	WarningQualityDegraded = "Polaris beta file understanding converted the original file into derived text context for a model that lacks native file/image support. This can reduce answer quality versus native multimodal processing."
	DefaultMaxTextChars    = 12000
)

type Input struct {
	FileID       string
	Sha256       string
	Filename     string
	MimeType     string
	Size         int64
	Data         []byte
	MaxTextChars int
	DerivedText  string
	Artifacts    []Artifact
}

type Artifact struct {
	Kind       ArtifactKind   `json:"kind,omitempty"`
	Processor  string         `json:"processor,omitempty"`
	Version    string         `json:"version,omitempty"`
	MimeType   string         `json:"mime_type,omitempty"`
	Text       string         `json:"text,omitempty"`
	Warning    string         `json:"warning,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Provenance []Provenance   `json:"provenance,omitempty"`
	Chunks     []Chunk        `json:"chunks,omitempty"`
	Embeddings []Embedding    `json:"embeddings,omitempty"`
	Pages      []Page         `json:"pages,omitempty"`
	Tables     []Table        `json:"tables,omitempty"`
	Forms      []FormField    `json:"forms,omitempty"`
}

type Provenance struct {
	FileID       string `json:"file_id,omitempty"`
	Sha256       string `json:"sha256,omitempty"`
	Filename     string `json:"filename,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	Processor    string `json:"processor,omitempty"`
	ArtifactKind string `json:"artifact_kind,omitempty"`
	Page         int    `json:"page,omitempty"`
	BlockID      string `json:"block_id,omitempty"`
	StartChar    int    `json:"start_char,omitempty"`
	EndChar      int    `json:"end_char,omitempty"`
}

type Chunk struct {
	ID            string         `json:"id,omitempty"`
	Text          string         `json:"text,omitempty"`
	StartChar     int            `json:"start_char,omitempty"`
	EndChar       int            `json:"end_char,omitempty"`
	TokenEstimate int            `json:"token_estimate,omitempty"`
	Source        Provenance     `json:"source,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type Embedding struct {
	ChunkID    string     `json:"chunk_id,omitempty"`
	Model      string     `json:"model,omitempty"`
	Dimensions int        `json:"dimensions,omitempty"`
	Vector     []float64  `json:"vector,omitempty"`
	Source     Provenance `json:"source,omitempty"`
}

type Page struct {
	Number     int            `json:"number,omitempty"`
	Text       string         `json:"text,omitempty"`
	Width      float64        `json:"width,omitempty"`
	Height     float64        `json:"height,omitempty"`
	Unit       string         `json:"unit,omitempty"`
	Blocks     []Block        `json:"blocks,omitempty"`
	Provenance Provenance     `json:"provenance,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type Block struct {
	ID         string         `json:"id,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Text       string         `json:"text,omitempty"`
	Confidence float64        `json:"confidence,omitempty"`
	Bounds     *Bounds        `json:"bounds,omitempty"`
	Provenance Provenance     `json:"provenance,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type Bounds struct {
	Left   float64 `json:"left,omitempty"`
	Top    float64 `json:"top,omitempty"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	Unit   string  `json:"unit,omitempty"`
}

type Table struct {
	ID         string         `json:"id,omitempty"`
	Page       int            `json:"page,omitempty"`
	Rows       [][]string     `json:"rows,omitempty"`
	Bounds     *Bounds        `json:"bounds,omitempty"`
	Confidence float64        `json:"confidence,omitempty"`
	Provenance Provenance     `json:"provenance,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type FormField struct {
	Key        string         `json:"key,omitempty"`
	Value      string         `json:"value,omitempty"`
	Page       int            `json:"page,omitempty"`
	Confidence float64        `json:"confidence,omitempty"`
	Bounds     *Bounds        `json:"bounds,omitempty"`
	Provenance Provenance     `json:"provenance,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type Processor interface {
	Name() string
	Version() string
	Supports(mimeType string) bool
	Process(ctx context.Context, input Input) (*Artifact, error)
}

type Registry struct {
	processors []Processor
}

func NewRegistry(processors ...Processor) *Registry {
	return &Registry{processors: append([]Processor(nil), processors...)}
}

func NewDefaultRegistry() *Registry {
	return NewRegistry(TextProcessor{}, PDFTextProcessor{}, OOXMLTextProcessor{}, ImageMetadataProcessor{})
}

func (r *Registry) Process(ctx context.Context, input Input) (*Artifact, error) {
	if r == nil {
		r = NewDefaultRegistry()
	}
	mimeType := CanonicalMIME(input.MimeType, input.Data)
	input.MimeType = mimeType
	processor := r.ProcessorFor(mimeType)
	if processor == nil {
		return nil, ErrUnsupportedMIME{MIMEType: mimeType}
	}
	return ProcessWithProcessor(ctx, processor, input)
}

func (r *Registry) ProcessorFor(mimeType string) Processor {
	if r == nil {
		r = NewDefaultRegistry()
	}
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	for _, processor := range r.processors {
		if processor.Supports(mimeType) {
			return processor
		}
	}
	return nil
}

func ProcessWithProcessor(ctx context.Context, processor Processor, input Input) (*Artifact, error) {
	if processor == nil {
		return nil, ErrUnsupportedMIME{MIMEType: input.MimeType}
	}
	artifact, err := processor.Process(ctx, input)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, fmt.Errorf("processor %s returned nil artifact", processor.Name())
	}
	if artifact.Processor == "" {
		artifact.Processor = processor.Name()
	}
	if artifact.Version == "" {
		artifact.Version = processor.Version()
	}
	if artifact.MimeType == "" {
		artifact.MimeType = input.MimeType
	}
	if artifact.Kind == "" {
		artifact.Kind = artifactKindFromMetadata(artifact.Metadata)
	}
	if artifact.Warning == "" {
		artifact.Warning = WarningQualityDegraded
	}
	artifact.Provenance = ensureArtifactProvenance(artifact, input)
	return artifact, nil
}

type ErrUnsupportedMIME struct {
	MIMEType string
}

func (e ErrUnsupportedMIME) Error() string {
	if e.MIMEType == "" {
		return "no file understanding processor supports this file"
	}
	return "no file understanding processor supports " + e.MIMEType
}

type TextProcessor struct{}

func (TextProcessor) Name() string {
	return "polaris_text_extract"
}

func (TextProcessor) Version() string {
	return "v1"
}

func (TextProcessor) Supports(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return strings.HasPrefix(mimeType, "text/") ||
		mimeType == "application/json" ||
		mimeType == "application/xml" ||
		mimeType == "application/x-yaml" ||
		mimeType == "application/yaml" ||
		mimeType == "application/javascript" ||
		mimeType == "application/x-ndjson"
}

func (TextProcessor) Process(_ context.Context, input Input) (*Artifact, error) {
	maxChars := input.MaxTextChars
	if maxChars <= 0 {
		maxChars = DefaultMaxTextChars
	}
	text := string(input.Data)
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "\uFFFD")
	}
	metadata := map[string]any{
		"artifact_kind":     string(ArtifactText),
		"bytes":             input.Size,
		"file_class":        string(ClassifyMIME(input.MimeType)),
		"representation":    "original_utf8_text",
		"source_preserving": true,
		"transformation":    "utf8_validation_only",
	}
	warning := WarningQualityDegraded
	runes := []rune(text)
	if len(runes) > maxChars {
		text = string(runes[:maxChars])
		metadata["truncated"] = true
		metadata["max_text_chars"] = maxChars
		warning += " Extracted text was truncated by files.understanding.max_text_chars."
	}
	return &Artifact{
		Processor: "polaris_text_extract",
		Version:   "v1",
		MimeType:  input.MimeType,
		Text:      text,
		Warning:   warning,
		Metadata:  metadata,
	}, nil
}

type PDFTextProcessor struct{}

func (PDFTextProcessor) Name() string {
	return "polaris_pdf_text_extract"
}

func (PDFTextProcessor) Version() string {
	return "v1"
}

func (PDFTextProcessor) Supports(mimeType string) bool {
	return strings.ToLower(strings.TrimSpace(mimeType)) == "application/pdf"
}

func (PDFTextProcessor) Process(_ context.Context, input Input) (*Artifact, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(input.Data), []byte("%PDF-")) {
		return nil, fmt.Errorf("invalid PDF header")
	}
	maxChars := input.MaxTextChars
	if maxChars <= 0 {
		maxChars = DefaultMaxTextChars
	}
	text, streamCount := extractPDFText(input.Data)
	text = normalizeSourceText(text)
	metadata := map[string]any{
		"artifact_kind":     string(ArtifactText),
		"bytes":             input.Size,
		"estimated_pages":   estimatePDFPages(input.Data),
		"file_class":        string(FileClassPDF),
		"representation":    "pdf_content_stream_text",
		"source_preserving": true,
		"text_streams":      streamCount,
	}
	warning := WarningQualityDegraded + " PDF extraction preserves born-digital content-stream text order where possible, but may miss scanned pages, custom font encodings, forms, annotations, or complex layout."
	if text == "" {
		text = "No extractable PDF text was found by the built-in best-effort processor. The PDF may be scanned, image-only, encrypted, or use an unsupported font encoding."
		metadata["text_empty"] = true
	}
	text, truncated := truncateRunes(text, maxChars)
	if truncated {
		metadata["truncated"] = true
		metadata["max_text_chars"] = maxChars
		warning += " Extracted text was truncated by files.understanding.max_text_chars."
	}
	return &Artifact{
		Processor: "polaris_pdf_text_extract",
		Version:   "v1",
		MimeType:  input.MimeType,
		Text:      text,
		Warning:   warning,
		Metadata:  metadata,
	}, nil
}

type OOXMLTextProcessor struct{}

const (
	MIMEDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	MIMEXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	MIMEPPTX = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
)

func (OOXMLTextProcessor) Name() string {
	return "polaris_ooxml_text_extract"
}

func (OOXMLTextProcessor) Version() string {
	return "v1"
}

func (OOXMLTextProcessor) Supports(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case MIMEDOCX, MIMEXLSX, MIMEPPTX:
		return true
	default:
		return false
	}
}

func (OOXMLTextProcessor) Process(_ context.Context, input Input) (*Artifact, error) {
	maxChars := input.MaxTextChars
	if maxChars <= 0 {
		maxChars = DefaultMaxTextChars
	}
	reader, err := zip.NewReader(bytes.NewReader(input.Data), int64(len(input.Data)))
	if err != nil {
		return nil, fmt.Errorf("open OOXML package: %w", err)
	}

	text, parts, warningDetail, err := extractOOXMLText(input.MimeType, reader.File)
	if err != nil {
		return nil, err
	}
	text = normalizeSourceText(text)
	metadata := map[string]any{
		"artifact_kind":     string(ArtifactText),
		"bytes":             input.Size,
		"file_class":        string(ClassifyMIME(input.MimeType)),
		"representation":    "ooxml_package_source_text",
		"source_preserving": true,
		"text_parts":        parts,
	}
	warning := WarningQualityDegraded + " OOXML extraction preserves source package text, document sections, slide notes, table cells, and spreadsheet cell references where possible."
	if warningDetail != "" {
		warning += " " + warningDetail
	}
	if text == "" {
		text = "No extractable OOXML text was found by the built-in processor."
		metadata["text_empty"] = true
	}
	text, truncated := truncateRunes(text, maxChars)
	if truncated {
		metadata["truncated"] = true
		metadata["max_text_chars"] = maxChars
		warning += " Extracted text was truncated by files.understanding.max_text_chars."
	}
	return &Artifact{
		Processor: "polaris_ooxml_text_extract",
		Version:   "v1",
		MimeType:  input.MimeType,
		Text:      text,
		Warning:   warning,
		Metadata:  metadata,
	}, nil
}

type ImageMetadataProcessor struct{}

func (ImageMetadataProcessor) Name() string {
	return "polaris_image_metadata"
}

func (ImageMetadataProcessor) Version() string {
	return "v1"
}

func (ImageMetadataProcessor) Supports(mimeType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/")
}

func (ImageMetadataProcessor) Process(_ context.Context, input Input) (*Artifact, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(input.Data))
	width, height := cfg.Width, cfg.Height
	if err != nil {
		var ok bool
		width, height, format, ok = decodeBasicImageMetadata(input.MimeType, input.Data)
		if !ok {
			return nil, fmt.Errorf("decode image metadata: %w", err)
		}
	}
	metadata := map[string]any{
		"artifact_kind":     string(ArtifactImageMetadata),
		"bytes":             input.Size,
		"file_class":        string(FileClassImage),
		"format":            format,
		"height":            height,
		"representation":    "image_container_metadata",
		"source_preserving": true,
		"width":             width,
	}
	text := fmt.Sprintf("Image metadata extracted by Polaris beta fallback.\nFilename: %s\nMIME type: %s\nFormat: %s\nDimensions: %dx%d pixels\n\nNo OCR or visual caption processor is configured in this build, so this derived context may omit important visual content.", input.Filename, input.MimeType, format, width, height)
	return &Artifact{
		Processor: "polaris_image_metadata",
		Version:   "v1",
		MimeType:  input.MimeType,
		Text:      text,
		Warning:   WarningQualityDegraded + " The built-in image fallback is metadata-only unless an OCR or caption processor is configured.",
		Metadata:  metadata,
	}, nil
}

func CanonicalMIME(mimeType string, data []byte) string {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	if mimeType != "" && mimeType != "application/octet-stream" {
		return mimeType
	}
	if len(data) > 0 {
		detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
		if detected != "" {
			return detected
		}
	}
	return mimeType
}

func MetadataJSON(metadata map[string]any) string {
	if len(metadata) == 0 {
		return "{}"
	}
	data, err := json.Marshal(metadata)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return "{}"
	}
	return string(data)
}

func CacheableArtifactJSON(artifact *Artifact) string {
	if artifact == nil {
		return "{}"
	}
	data, err := json.Marshal(artifact)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return "{}"
	}
	var copied Artifact
	if err := json.Unmarshal(data, &copied); err != nil {
		return "{}"
	}
	copied.Provenance = scrubProvenanceSlice(copied.Provenance)
	for i := range copied.Chunks {
		scrubProvenance(&copied.Chunks[i].Source)
	}
	for i := range copied.Embeddings {
		scrubProvenance(&copied.Embeddings[i].Source)
	}
	for i := range copied.Pages {
		scrubProvenance(&copied.Pages[i].Provenance)
		for j := range copied.Pages[i].Blocks {
			scrubProvenance(&copied.Pages[i].Blocks[j].Provenance)
		}
	}
	for i := range copied.Tables {
		scrubProvenance(&copied.Tables[i].Provenance)
	}
	for i := range copied.Forms {
		scrubProvenance(&copied.Forms[i].Provenance)
	}
	data, err = json.Marshal(copied)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return "{}"
	}
	return string(data)
}

func ArtifactFromJSON(raw string) (*Artifact, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return nil, false
	}
	var artifact Artifact
	if err := json.Unmarshal([]byte(raw), &artifact); err != nil {
		return nil, false
	}
	return &artifact, true
}

func RebindArtifactProvenance(artifact *Artifact, input Input) {
	if artifact == nil {
		return
	}
	artifact.Provenance = ensureArtifactProvenance(artifact, input)
	for i := range artifact.Provenance {
		bindProvenanceSource(&artifact.Provenance[i], input)
	}
	for i := range artifact.Chunks {
		bindProvenanceSource(&artifact.Chunks[i].Source, input)
	}
	for i := range artifact.Embeddings {
		bindProvenanceSource(&artifact.Embeddings[i].Source, input)
	}
	for i := range artifact.Pages {
		bindProvenanceSource(&artifact.Pages[i].Provenance, input)
		for j := range artifact.Pages[i].Blocks {
			bindProvenanceSource(&artifact.Pages[i].Blocks[j].Provenance, input)
		}
	}
	for i := range artifact.Tables {
		bindProvenanceSource(&artifact.Tables[i].Provenance, input)
	}
	for i := range artifact.Forms {
		bindProvenanceSource(&artifact.Forms[i].Provenance, input)
	}
}

func artifactKindFromMetadata(metadata map[string]any) ArtifactKind {
	if len(metadata) == 0 {
		return ArtifactText
	}
	if raw, ok := metadata["artifact_kind"]; ok {
		if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
			return ArtifactKind(strings.TrimSpace(value))
		}
	}
	return ArtifactText
}

func ensureArtifactProvenance(artifact *Artifact, input Input) []Provenance {
	if artifact == nil {
		return nil
	}
	if len(artifact.Provenance) > 0 {
		out := append([]Provenance(nil), artifact.Provenance...)
		for i := range out {
			fillProvenanceDefaults(&out[i], artifact, input)
		}
		return out
	}
	provenance := Provenance{
		StartChar: 0,
		EndChar:   len([]rune(artifact.Text)),
	}
	fillProvenanceDefaults(&provenance, artifact, input)
	return []Provenance{provenance}
}

func scrubProvenanceSlice(values []Provenance) []Provenance {
	if len(values) == 0 {
		return nil
	}
	out := append([]Provenance(nil), values...)
	for i := range out {
		scrubProvenance(&out[i])
	}
	return out
}

func scrubProvenance(provenance *Provenance) {
	if provenance == nil {
		return
	}
	provenance.FileID = ""
	provenance.Sha256 = ""
	provenance.Filename = ""
	provenance.MimeType = ""
}

func bindProvenanceSource(provenance *Provenance, input Input) {
	if provenance == nil {
		return
	}
	provenance.FileID = input.FileID
	provenance.Sha256 = input.Sha256
	provenance.Filename = input.Filename
	provenance.MimeType = input.MimeType
}

func fillProvenanceDefaults(provenance *Provenance, artifact *Artifact, input Input) {
	if provenance == nil || artifact == nil {
		return
	}
	if provenance.FileID == "" {
		provenance.FileID = input.FileID
	}
	if provenance.Sha256 == "" {
		provenance.Sha256 = input.Sha256
	}
	if provenance.Filename == "" {
		provenance.Filename = input.Filename
	}
	if provenance.MimeType == "" {
		provenance.MimeType = firstString(input.MimeType, artifact.MimeType)
	}
	if provenance.Processor == "" {
		provenance.Processor = artifact.Processor
	}
	if provenance.ArtifactKind == "" {
		provenance.ArtifactKind = string(artifact.Kind)
	}
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func HumanFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "unnamed file"
	}
	return filepath.Base(filename)
}

func truncateRunes(text string, maxChars int) (string, bool) {
	if maxChars <= 0 {
		maxChars = DefaultMaxTextChars
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text, false
	}
	return string(runes[:maxChars]), true
}

func normalizeSourceText(text string) string {
	text = strings.ToValidUTF8(text, "\uFFFD")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	var b strings.Builder
	blankLines := 0
	for _, line := range lines {
		line = normalizeSourceLine(line)
		if strings.TrimSpace(line) == "" {
			if b.Len() > 0 && blankLines == 0 {
				b.WriteByte('\n')
				blankLines++
			}
			continue
		}
		if b.Len() > 0 && lastBuilderByte(&b) != '\n' {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		blankLines = 0
	}
	return strings.TrimSpace(b.String())
}

func normalizeSourceLine(line string) string {
	line = strings.ToValidUTF8(line, "\uFFFD")
	var b strings.Builder
	b.Grow(len(line))
	lastSpace := false
	for _, r := range line {
		if r == utf8.RuneError {
			continue
		}
		if r == '\t' {
			for b.Len() > 0 && lastBuilderByte(&b) == ' ' {
				s := b.String()
				b.Reset()
				b.WriteString(s[:len(s)-1])
			}
			b.WriteByte('\t')
			lastSpace = false
			continue
		}
		if unicode.IsSpace(r) {
			if !lastSpace && b.Len() > 0 && lastBuilderByte(&b) != '\t' {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

func appendSourceTextRun(b *strings.Builder, value string) {
	if value == "" {
		return
	}
	value = strings.ToValidUTF8(value, "\uFFFD")
	b.WriteString(value)
}

func appendSourceLineBreak(b *strings.Builder) {
	if b.Len() == 0 || lastBuilderByte(b) == '\n' {
		return
	}
	b.WriteByte('\n')
}

func appendSourceCellBreak(b *strings.Builder) {
	if b.Len() == 0 {
		return
	}
	trimmed := strings.TrimRight(b.String(), " \n")
	b.Reset()
	b.WriteString(trimmed)
	if b.Len() == 0 {
		return
	}
	last := lastBuilderByte(b)
	if last != '\t' && last != '\n' {
		b.WriteByte('\t')
	}
}

func lastBuilderByte(b *strings.Builder) byte {
	if b == nil || b.Len() == 0 {
		return 0
	}
	s := b.String()
	return s[len(s)-1]
}

func estimatePDFPages(data []byte) int {
	count := 0
	needle := []byte("/Type /Page")
	offset := 0
	for {
		idx := bytes.Index(data[offset:], needle)
		if idx < 0 {
			break
		}
		pos := offset + idx + len(needle)
		if pos >= len(data) || data[pos] != 's' {
			count++
		}
		offset = pos
	}
	return count
}

func extractPDFText(data []byte) (string, int) {
	var b strings.Builder
	streamCount := 0
	offset := 0
	for {
		streamRel := bytes.Index(data[offset:], []byte("stream"))
		if streamRel < 0 {
			break
		}
		streamPos := offset + streamRel
		endRel := bytes.Index(data[streamPos:], []byte("endstream"))
		if endRel < 0 {
			break
		}
		endPos := streamPos + endRel
		dictStart := bytes.LastIndex(data[:streamPos], []byte("<<"))
		dict := []byte(nil)
		if dictStart >= 0 {
			dict = data[dictStart:streamPos]
		}
		contentStart := streamPos + len("stream")
		if contentStart < len(data) && data[contentStart] == '\r' {
			contentStart++
		}
		if contentStart < len(data) && data[contentStart] == '\n' {
			contentStart++
		}
		contentEnd := endPos
		for contentEnd > contentStart && (data[contentEnd-1] == '\r' || data[contentEnd-1] == '\n') {
			contentEnd--
		}
		if contentStart < contentEnd && !bytes.Contains(dict, []byte("/Subtype /Image")) {
			content := append([]byte(nil), data[contentStart:contentEnd]...)
			if bytes.Contains(dict, []byte("/FlateDecode")) {
				if inflated, err := inflateZlib(content); err == nil {
					content = inflated
				}
			}
			text := extractPDFContentText(content)
			if strings.TrimSpace(text) != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(text)
				streamCount++
			}
		}
		offset = endPos + len("endstream")
	}
	return b.String(), streamCount
}

func inflateZlib(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()
	return io.ReadAll(reader)
}

func extractPDFContentText(data []byte) string {
	var b strings.Builder
	offset := 0
	for {
		btRel := bytes.Index(data[offset:], []byte("BT"))
		if btRel < 0 {
			break
		}
		blockStart := offset + btRel + len("BT")
		etRel := bytes.Index(data[blockStart:], []byte("ET"))
		if etRel < 0 {
			break
		}
		block := data[blockStart : blockStart+etRel]
		if text := extractPDFTextBlock(block); strings.TrimSpace(text) != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(text)
		}
		offset = blockStart + etRel + len("ET")
	}
	return b.String()
}

func extractPDFTextBlock(block []byte) string {
	var b strings.Builder
	for i := 0; i < len(block); {
		switch block[i] {
		case '(':
			value, next := parsePDFLiteralString(block, i)
			appendPDFText(&b, value)
			i = next
		case '<':
			if i+1 < len(block) && block[i+1] == '<' {
				i += 2
				continue
			}
			value, next := parsePDFHexString(block, i)
			appendPDFText(&b, value)
			i = next
		default:
			if isPDFNumberStart(block, i) {
				value, next := parsePDFNumber(block, i)
				if math.Abs(value) > 5000 {
					b.WriteByte('\n')
				} else if math.Abs(value) > 250 {
					b.WriteByte(' ')
				}
				i = next
				continue
			}
			i++
		}
	}
	return b.String()
}

func appendPDFText(b *strings.Builder, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if b.Len() > 0 {
		last := b.String()[b.Len()-1]
		if last != ' ' && last != '\n' && needsPDFTextGap(value) {
			b.WriteByte(' ')
		}
	}
	b.WriteString(value)
}

func needsPDFTextGap(value string) bool {
	if value == "" {
		return false
	}
	r := []rune(value)[0]
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isPDFNumberStart(data []byte, pos int) bool {
	c := data[pos]
	if c != '+' && c != '-' && c != '.' && (c < '0' || c > '9') {
		return false
	}
	if pos > 0 {
		prev := data[pos-1]
		if (prev >= '0' && prev <= '9') || prev == '.' || unicode.IsLetter(rune(prev)) {
			return false
		}
	}
	return true
}

func parsePDFNumber(data []byte, pos int) (float64, int) {
	end := pos
	for end < len(data) {
		c := data[end]
		if c != '+' && c != '-' && c != '.' && (c < '0' || c > '9') {
			break
		}
		end++
	}
	value, _ := strconv.ParseFloat(string(data[pos:end]), 64)
	return value, end
}

func parsePDFLiteralString(data []byte, pos int) (string, int) {
	depth := 1
	var out []byte
	i := pos + 1
	for i < len(data) && depth > 0 {
		c := data[i]
		if c == '\\' && i+1 < len(data) {
			next := data[i+1]
			switch next {
			case 'n':
				out = append(out, '\n')
				i += 2
			case 'r':
				out = append(out, '\r')
				i += 2
			case 't':
				out = append(out, '\t')
				i += 2
			case 'b':
				out = append(out, '\b')
				i += 2
			case 'f':
				out = append(out, '\f')
				i += 2
			case '(', ')', '\\':
				out = append(out, next)
				i += 2
			case '\r', '\n':
				i += 2
				if next == '\r' && i < len(data) && data[i] == '\n' {
					i++
				}
			default:
				if next >= '0' && next <= '7' {
					value := 0
					digits := 0
					j := i + 1
					for j < len(data) && digits < 3 && data[j] >= '0' && data[j] <= '7' {
						value = value*8 + int(data[j]-'0')
						j++
						digits++
					}
					out = append(out, byte(value))
					i = j
				} else {
					out = append(out, next)
					i += 2
				}
			}
			continue
		}
		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth > 0 {
				out = append(out, c)
			}
		default:
			out = append(out, c)
		}
		i++
	}
	return decodePDFString(out), i
}

func parsePDFHexString(data []byte, pos int) (string, int) {
	end := pos + 1
	for end < len(data) && data[end] != '>' {
		end++
	}
	raw := make([]byte, 0, end-pos)
	for _, c := range data[pos+1 : end] {
		if !unicode.IsSpace(rune(c)) {
			raw = append(raw, c)
		}
	}
	if len(raw)%2 == 1 {
		raw = append(raw, '0')
	}
	decoded, err := hex.DecodeString(string(raw))
	if err != nil {
		if end < len(data) {
			return "", end + 1
		}
		return "", end
	}
	if end < len(data) {
		end++
	}
	return decodePDFString(decoded), end
}

func decodePDFString(data []byte) string {
	if len(data) >= 2 {
		switch {
		case data[0] == 0xfe && data[1] == 0xff:
			return decodeUTF16(data[2:], binary.BigEndian)
		case data[0] == 0xff && data[1] == 0xfe:
			return decodeUTF16(data[2:], binary.LittleEndian)
		}
	}
	return strings.ToValidUTF8(string(data), "\uFFFD")
}

func decodeUTF16(data []byte, order binary.ByteOrder) string {
	runes := make([]rune, 0, len(data)/2)
	for len(data) >= 2 {
		runes = append(runes, rune(order.Uint16(data[:2])))
		data = data[2:]
	}
	return string(runes)
}

func extractOOXMLText(mimeType string, files []*zip.File) (string, int, string, error) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case MIMEDOCX:
		return extractDOCXText(files)
	case MIMEPPTX:
		return extractPPTXText(files)
	case MIMEXLSX:
		return extractXLSXText(files)
	default:
		return "", 0, "", ErrUnsupportedMIME{MIMEType: mimeType}
	}
}

func extractDOCXText(files []*zip.File) (string, int, string, error) {
	var b strings.Builder
	parts := 0
	writeDocxPart := func(label string, file *zip.File) {
		if file == nil {
			return
		}
		text, err := extractWordprocessingXMLText(file)
		if err != nil || strings.TrimSpace(text) == "" {
			return
		}
		writeLabeledSourceBlock(&b, label, text)
		parts++
	}
	writeDocxPart("Document", zipFileByName(files, "word/document.xml"))
	for _, file := range zipFilesMatching(files, func(name string) bool {
		return strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml")
	}) {
		writeDocxPart("Header "+filepath.Base(file.Name), file)
	}
	for _, file := range zipFilesMatching(files, func(name string) bool {
		return strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml")
	}) {
		writeDocxPart("Footer "+filepath.Base(file.Name), file)
	}
	for _, name := range []string{"word/footnotes.xml", "word/endnotes.xml", "word/comments.xml"} {
		writeDocxPart(strings.TrimSuffix(strings.TrimPrefix(name, "word/"), ".xml"), zipFileByName(files, name))
	}
	if parts == 0 {
		return "", 0, "", fmt.Errorf("OOXML package does not contain supported DOCX text parts")
	}
	return b.String(), parts, "Tracked changes, revision state, embedded images, charts, and exact visual layout are not reconstructed by the built-in processor.", nil
}

func extractPPTXText(files []*zip.File) (string, int, string, error) {
	var b strings.Builder
	parts := 0
	for _, file := range zipFilesMatching(files, func(name string) bool {
		return strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml")
	}) {
		text, err := extractDrawingXMLText(file)
		if err != nil || strings.TrimSpace(text) == "" {
			continue
		}
		writeLabeledSourceBlock(&b, "Slide "+ooxmlPartOrdinal(file.Name), text)
		parts++
	}
	for _, file := range zipFilesMatching(files, func(name string) bool {
		return strings.HasPrefix(name, "ppt/notesSlides/notesSlide") && strings.HasSuffix(name, ".xml")
	}) {
		text, err := extractDrawingXMLText(file)
		if err != nil || strings.TrimSpace(text) == "" {
			continue
		}
		writeLabeledSourceBlock(&b, "Notes "+ooxmlPartOrdinal(file.Name), text)
		parts++
	}
	if parts == 0 {
		return "", 0, "", fmt.Errorf("OOXML package does not contain supported PPTX text parts")
	}
	return b.String(), parts, "Embedded images, charts, speaker timing, animations, and exact slide layout are not reconstructed by the built-in processor.", nil
}

func extractXLSXText(files []*zip.File) (string, int, string, error) {
	sharedStrings, _ := extractXLSXSharedStrings(zipFileByName(files, "xl/sharedStrings.xml"))
	var b strings.Builder
	parts := 0
	for _, file := range zipFilesMatching(files, func(name string) bool {
		return strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml")
	}) {
		text, rows, err := extractXLSXSheetText(file, sharedStrings)
		if err != nil || rows == 0 || strings.TrimSpace(text) == "" {
			continue
		}
		writeLabeledSourceBlock(&b, "Sheet "+strings.TrimSuffix(filepath.Base(file.Name), ".xml"), text)
		parts++
	}
	if parts == 0 {
		return "", 0, "", fmt.Errorf("OOXML package does not contain supported XLSX sheet text")
	}
	return b.String(), parts, "Spreadsheet formulas and cached values are preserved when present, but styles, date formatting, charts, pivots, filters, and workbook calculation semantics are not reconstructed by the built-in processor.", nil
}

func extractWordprocessingXMLText(file *zip.File) (string, error) {
	reader, err := file.Open()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = reader.Close()
	}()
	decoder := xml.NewDecoder(reader)
	var b strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "t":
				var value string
				if err := decoder.DecodeElement(&value, &token); err != nil {
					return "", err
				}
				appendSourceTextRun(&b, value)
			case "tab":
				b.WriteByte('\t')
			case "br", "cr":
				appendSourceLineBreak(&b)
			}
		case xml.EndElement:
			switch token.Name.Local {
			case "p":
				appendSourceLineBreak(&b)
			case "tc":
				appendSourceCellBreak(&b)
			case "tr":
				appendSourceLineBreak(&b)
			}
		}
	}
	return b.String(), nil
}

func extractDrawingXMLText(file *zip.File) (string, error) {
	reader, err := file.Open()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = reader.Close()
	}()
	decoder := xml.NewDecoder(reader)
	var b strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "t":
				var value string
				if err := decoder.DecodeElement(&value, &token); err != nil {
					return "", err
				}
				appendSourceTextRun(&b, value)
			case "tab":
				b.WriteByte('\t')
			case "br":
				appendSourceLineBreak(&b)
			}
		case xml.EndElement:
			switch token.Name.Local {
			case "p":
				appendSourceLineBreak(&b)
			case "tc":
				appendSourceCellBreak(&b)
			case "tr":
				appendSourceLineBreak(&b)
			}
		}
	}
	return b.String(), nil
}

func extractXLSXSharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()
	decoder := xml.NewDecoder(reader)
	var values []string
	var current *strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "si":
				current = &strings.Builder{}
			case "t":
				var value string
				if err := decoder.DecodeElement(&value, &token); err != nil {
					return nil, err
				}
				if current != nil {
					appendSourceTextRun(current, value)
				}
			}
		case xml.EndElement:
			if token.Name.Local == "si" && current != nil {
				values = append(values, normalizeSourceLine(current.String()))
				current = nil
			}
		}
	}
	return values, nil
}

type xlsxCell struct {
	ref     string
	typ     string
	value   string
	formula string
	inline  strings.Builder
}

func extractXLSXSheetText(file *zip.File, sharedStrings []string) (string, int, error) {
	reader, err := file.Open()
	if err != nil {
		return "", 0, err
	}
	defer func() {
		_ = reader.Close()
	}()
	decoder := xml.NewDecoder(reader)
	var b strings.Builder
	var row []string
	var cell *xlsxCell
	rows := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", 0, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "row":
				row = nil
			case "c":
				cell = &xlsxCell{
					ref: attrValue(token.Attr, "r"),
					typ: attrValue(token.Attr, "t"),
				}
			case "v":
				var value string
				if err := decoder.DecodeElement(&value, &token); err != nil {
					return "", 0, err
				}
				if cell != nil {
					cell.value = strings.TrimSpace(value)
				}
			case "f":
				var value string
				if err := decoder.DecodeElement(&value, &token); err != nil {
					return "", 0, err
				}
				if cell != nil {
					cell.formula = strings.TrimSpace(value)
				}
			case "t":
				var value string
				if err := decoder.DecodeElement(&value, &token); err != nil {
					return "", 0, err
				}
				if cell != nil {
					appendSourceTextRun(&cell.inline, value)
				}
			}
		case xml.EndElement:
			switch token.Name.Local {
			case "c":
				if cell != nil {
					if text := formatXLSXCell(*cell, sharedStrings); text != "" {
						row = append(row, text)
					}
					cell = nil
				}
			case "row":
				if len(row) > 0 {
					if b.Len() > 0 {
						b.WriteByte('\n')
					}
					b.WriteString(strings.Join(row, "\t"))
					rows++
				}
				row = nil
			}
		}
	}
	return b.String(), rows, nil
}

func formatXLSXCell(cell xlsxCell, sharedStrings []string) string {
	value := strings.TrimSpace(cell.value)
	switch cell.typ {
	case "s":
		index, err := strconv.Atoi(value)
		if err == nil && index >= 0 && index < len(sharedStrings) {
			value = sharedStrings[index]
		}
	case "inlineStr":
		value = normalizeSourceLine(cell.inline.String())
	case "b":
		switch value {
		case "1":
			value = "TRUE"
		case "0":
			value = "FALSE"
		}
	case "str":
		if inline := normalizeSourceLine(cell.inline.String()); inline != "" {
			value = inline
		}
	}
	if formula := strings.TrimSpace(cell.formula); formula != "" {
		if value != "" {
			value = "formula=" + formula + " value=" + value
		} else {
			value = "formula=" + formula
		}
	}
	value = normalizeSourceLine(value)
	if value == "" {
		return ""
	}
	if cell.ref == "" {
		return value
	}
	return cell.ref + "=" + value
}

func writeLabeledSourceBlock(b *strings.Builder, label, text string) {
	text = normalizeSourceText(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(strings.TrimSpace(label))
	b.WriteString(":\n")
	b.WriteString(text)
}

func zipFileByName(files []*zip.File, name string) *zip.File {
	name = strings.TrimPrefix(name, "/")
	for _, file := range files {
		if strings.TrimPrefix(file.Name, "/") == name {
			return file
		}
	}
	return nil
}

func zipFilesMatching(files []*zip.File, match func(string) bool) []*zip.File {
	var selected []*zip.File
	for _, file := range files {
		name := strings.TrimPrefix(file.Name, "/")
		if match(name) {
			selected = append(selected, file)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		return naturalOOXMLLess(selected[i].Name, selected[j].Name)
	})
	return selected
}

func naturalOOXMLLess(a, b string) bool {
	an, aok := trailingNumberBeforeExt(a)
	bn, bok := trailingNumberBeforeExt(b)
	if aok && bok && an != bn {
		return an < bn
	}
	return a < b
}

func ooxmlPartOrdinal(name string) string {
	n, ok := trailingNumberBeforeExt(name)
	if !ok {
		return strings.TrimSuffix(filepath.Base(name), ".xml")
	}
	return strconv.Itoa(n)
}

func trailingNumberBeforeExt(name string) (int, bool) {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	end := len(base)
	start := end
	for start > 0 && base[start-1] >= '0' && base[start-1] <= '9' {
		start--
	}
	if start == end {
		return 0, false
	}
	n, err := strconv.Atoi(base[start:end])
	return n, err == nil
}

func attrValue(attrs []xml.Attr, local string) string {
	for _, attr := range attrs {
		if attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}

func decodeBasicImageMetadata(mimeType string, data []byte) (int, int, string, bool) {
	if width, height, ok := decodeWebPMetadata(data); ok {
		return width, height, "webp", true
	}
	if width, height, ok := decodeBMPMetadata(data); ok {
		return width, height, "bmp", true
	}
	if width, height, ok := decodeTIFFMetadata(data); ok {
		return width, height, "tiff", true
	}
	if strings.EqualFold(strings.TrimSpace(mimeType), "image/svg+xml") {
		if width, height, ok := decodeSVGMetadata(data); ok {
			return width, height, "svg", true
		}
	}
	return 0, 0, "", false
}

func decodeWebPMetadata(data []byte) (int, int, bool) {
	if len(data) < 30 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0, false
	}
	pos := 12
	for pos+8 <= len(data) {
		chunk := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		payload := pos + 8
		if payload+size > len(data) {
			return 0, 0, false
		}
		switch chunk {
		case "VP8X":
			if size >= 10 {
				width := 1 + int(data[payload+4]) + int(data[payload+5])<<8 + int(data[payload+6])<<16
				height := 1 + int(data[payload+7]) + int(data[payload+8])<<8 + int(data[payload+9])<<16
				return width, height, true
			}
		case "VP8L":
			if size >= 5 && data[payload] == 0x2f {
				b1, b2, b3, b4 := data[payload+1], data[payload+2], data[payload+3], data[payload+4]
				width := 1 + int(b1) + int(b2&0x3f)<<8
				height := 1 + int(b2>>6) + int(b3)<<2 + int(b4&0x0f)<<10
				return width, height, true
			}
		case "VP8 ":
			if size >= 10 && data[payload+3] == 0x9d && data[payload+4] == 0x01 && data[payload+5] == 0x2a {
				width := int(binary.LittleEndian.Uint16(data[payload+6:payload+8]) & 0x3fff)
				height := int(binary.LittleEndian.Uint16(data[payload+8:payload+10]) & 0x3fff)
				return width, height, true
			}
		}
		pos = payload + size
		if pos%2 == 1 {
			pos++
		}
	}
	return 0, 0, false
}

func decodeBMPMetadata(data []byte) (int, int, bool) {
	if len(data) < 26 || string(data[:2]) != "BM" {
		return 0, 0, false
	}
	width, ok := decodeBMPSignedDimension(data[18:22])
	if !ok {
		return 0, 0, false
	}
	height, ok := decodeBMPSignedDimension(data[22:26])
	if !ok {
		return 0, 0, false
	}
	return width, height, true
}

func decodeBMPSignedDimension(data []byte) (int, bool) {
	if len(data) < 4 {
		return 0, false
	}
	raw := binary.LittleEndian.Uint32(data)
	value := int64(raw)
	if raw&(1<<31) != 0 {
		value -= 1 << 32
	}
	if value < 0 {
		value = -value
	}
	if value == 0 || value > int64(math.MaxInt) {
		return 0, false
	}
	return int(value), true
}

func decodeTIFFMetadata(data []byte) (int, int, bool) {
	if len(data) < 8 {
		return 0, 0, false
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, 0, false
	}
	if order.Uint16(data[2:4]) != 42 {
		return 0, 0, false
	}
	ifdOffset := int(order.Uint32(data[4:8]))
	if ifdOffset < 0 || ifdOffset+2 > len(data) {
		return 0, 0, false
	}
	entries := int(order.Uint16(data[ifdOffset : ifdOffset+2]))
	pos := ifdOffset + 2
	width, height := 0, 0
	for i := 0; i < entries && pos+12 <= len(data); i++ {
		tag := order.Uint16(data[pos : pos+2])
		typ := order.Uint16(data[pos+2 : pos+4])
		count := order.Uint32(data[pos+4 : pos+8])
		value := data[pos+8 : pos+12]
		if count == 1 && (typ == 3 || typ == 4) {
			n := 0
			if typ == 3 {
				n = int(order.Uint16(value[:2]))
			} else {
				n = int(order.Uint32(value))
			}
			switch tag {
			case 256:
				width = n
			case 257:
				height = n
			}
		}
		pos += 12
	}
	return width, height, width > 0 && height > 0
}

func decodeSVGMetadata(data []byte) (int, int, bool) {
	type svgRoot struct {
		XMLName xml.Name `xml:"svg"`
		Width   string   `xml:"width,attr"`
		Height  string   `xml:"height,attr"`
		ViewBox string   `xml:"viewBox,attr"`
	}
	var root svgRoot
	if err := xml.Unmarshal(data, &root); err != nil || root.XMLName.Local != "svg" {
		return 0, 0, false
	}
	width := parseSVGLength(root.Width)
	height := parseSVGLength(root.Height)
	if (width <= 0 || height <= 0) && root.ViewBox != "" {
		fields := strings.Fields(strings.ReplaceAll(root.ViewBox, ",", " "))
		if len(fields) == 4 {
			width = parseSVGLength(fields[2])
			height = parseSVGLength(fields[3])
		}
	}
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return int(math.Round(width)), int(math.Round(height)), true
}

func parseSVGLength(value string) float64 {
	value = strings.TrimSpace(value)
	value = strings.TrimRightFunc(value, func(r rune) bool {
		return unicode.IsLetter(r) || r == '%'
	})
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}
