package understanding

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPDFTextProcessorExtractsCompressedText(t *testing.T) {
	pdf := minimalPDFBytes(t, "Hello from compressed PDF")
	artifact, err := ProcessWithProcessor(context.Background(), PDFTextProcessor{}, Input{
		Filename:     "sample.pdf",
		MimeType:     "application/pdf",
		Size:         int64(len(pdf)),
		Data:         pdf,
		MaxTextChars: 2000,
	})
	if err != nil {
		t.Fatalf("ProcessWithProcessor(PDF) error = %v", err)
	}
	if artifact.Processor != "polaris_pdf_text_extract" {
		t.Fatalf("unexpected processor %q", artifact.Processor)
	}
	if !strings.Contains(artifact.Text, "Hello from compressed PDF") {
		t.Fatalf("expected extracted PDF text, got %q", artifact.Text)
	}
}

func TestOOXMLTextProcessorExtractsOfficeText(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		mimeType string
		files    map[string]string
		wantAll  []string
	}{
		{
			name:     "docx",
			filename: "sample.docx",
			mimeType: MIMEDOCX,
			files: map[string]string{
				"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
				"word/document.xml":   `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hello from DOCX</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>Left cell</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Right cell</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`,
				"word/comments.xml":   `<?xml version="1.0"?><w:comments xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:comment><w:p><w:r><w:t>Reviewer comment</w:t></w:r></w:p></w:comment></w:comments>`,
			},
			wantAll: []string{"Document:", "Hello from DOCX", "Left cell\tRight cell", "comments:", "Reviewer comment"},
		},
		{
			name:     "pptx",
			filename: "sample.pptx",
			mimeType: MIMEPPTX,
			files: map[string]string{
				"[Content_Types].xml":             `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
				"ppt/presentation.xml":            `<?xml version="1.0"?><p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"></p:presentation>`,
				"ppt/slides/slide1.xml":           `<?xml version="1.0"?><p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>Hello from PPTX</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`,
				"ppt/notesSlides/notesSlide1.xml": `<?xml version="1.0"?><p:notes xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>Speaker note</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:notes>`,
			},
			wantAll: []string{"Slide 1:", "Hello from PPTX", "Notes 1:", "Speaker note"},
		},
		{
			name:     "xlsx",
			filename: "sample.xlsx",
			mimeType: MIMEXLSX,
			files: map[string]string{
				"[Content_Types].xml":      `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
				"xl/workbook.xml":          `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"></workbook>`,
				"xl/sharedStrings.xml":     `<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Hello from XLSX</t></si></sst>`,
				"xl/worksheets/sheet1.xml": `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1"><f>SUM(40,2)</f><v>42</v></c></row></sheetData></worksheet>`,
			},
			wantAll: []string{"Sheet sheet1:", "A1=Hello from XLSX", "B1=formula=SUM(40,2) value=42"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := minimalOOXMLPackage(t, tc.files)
			artifact, err := ProcessWithProcessor(context.Background(), OOXMLTextProcessor{}, Input{
				Filename:     tc.filename,
				MimeType:     tc.mimeType,
				Size:         int64(len(data)),
				Data:         data,
				MaxTextChars: 2000,
			})
			if err != nil {
				t.Fatalf("ProcessWithProcessor(%s) error = %v", tc.name, err)
			}
			if artifact.Processor != "polaris_ooxml_text_extract" {
				t.Fatalf("unexpected processor %q", artifact.Processor)
			}
			for _, want := range tc.wantAll {
				if !strings.Contains(artifact.Text, want) {
					t.Fatalf("expected %q in extracted %s text, got %q", want, tc.name, artifact.Text)
				}
			}
			if artifact.Metadata["source_preserving"] != true {
				t.Fatalf("expected source-preserving metadata, got %#v", artifact.Metadata)
			}
		})
	}
}

func TestImageMetadataProcessorSupportsPopularHeaders(t *testing.T) {
	cases := []struct {
		name     string
		mimeType string
		data     []byte
		want     string
	}{
		{name: "png", mimeType: "image/png", data: tinyPNGBytes(t), want: "Dimensions: 1x1 pixels"},
		{name: "webp", mimeType: "image/webp", data: minimalWebPVP8X(2, 3), want: "Dimensions: 2x3 pixels"},
		{name: "svg", mimeType: "image/svg+xml", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="20"></svg>`), want: "Dimensions: 10x20 pixels"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := ProcessWithProcessor(context.Background(), ImageMetadataProcessor{}, Input{
				Filename: tc.name,
				MimeType: tc.mimeType,
				Size:     int64(len(tc.data)),
				Data:     tc.data,
			})
			if err != nil {
				t.Fatalf("ProcessWithProcessor(%s) error = %v", tc.name, err)
			}
			if !strings.Contains(artifact.Text, tc.want) {
				t.Fatalf("expected %q in image metadata, got %q", tc.want, artifact.Text)
			}
		})
	}
}

func minimalPDFBytes(t *testing.T, text string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := fmt.Fprintf(zw, "BT /F1 12 Tf 72 720 Td [(%s)]TJ ET", text); err != nil {
		t.Fatalf("write compressed PDF stream: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zlib writer: %v", err)
	}
	var pdf bytes.Buffer
	_, _ = pdf.WriteString("%PDF-1.4\n")
	_, _ = pdf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	_, _ = pdf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	_, _ = pdf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>\nendobj\n")
	_, _ = fmt.Fprintf(&pdf, "4 0 obj\n<< /Filter /FlateDecode /Length %d >>\nstream\n", compressed.Len())
	_, _ = pdf.Write(compressed.Bytes())
	_, _ = pdf.WriteString("\nendstream\nendobj\n%%EOF\n")
	return pdf.Bytes()
}

func minimalOOXMLPackage(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func minimalWebPVP8X(width, height int) []byte {
	payload := []byte{0, 0, 0, 0, byte(width - 1), byte((width - 1) >> 8), byte((width - 1) >> 16), byte(height - 1), byte((height - 1) >> 8), byte((height - 1) >> 16)}
	size := 4 + 8 + len(payload)
	data := []byte{'R', 'I', 'F', 'F', byte(size), byte(size >> 8), byte(size >> 16), byte(size >> 24), 'W', 'E', 'B', 'P', 'V', 'P', '8', 'X', byte(len(payload)), 0, 0, 0}
	data = append(data, payload...)
	return data
}

func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	return []byte{
		0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x04, 0x00, 0x00, 0x00, 0xb5, 0x1c, 0x0c,
		0x02, 0x00, 0x00, 0x00, 0x0b, 'I', 'D', 'A', 'T',
		0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00, 0x03,
		0x03, 0x02, 0x00, 0xef, 0xbf, 0xa7, 0xdb, 0x00,
		0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42,
		0x60, 0x82,
	}
}
