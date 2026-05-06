package understanding

import "testing"

func TestClassifyMIME(t *testing.T) {
	cases := []struct {
		mimeType string
		want     FileClass
	}{
		{mimeType: "text/markdown", want: FileClassText},
		{mimeType: "image/png", want: FileClassImage},
		{mimeType: "application/pdf", want: FileClassPDF},
		{mimeType: MIMEDOCX, want: FileClassOfficeDocument},
		{mimeType: MIMEXLSX, want: FileClassOfficeSpreadsheet},
		{mimeType: MIMEPPTX, want: FileClassOfficePresentation},
		{mimeType: "audio/mpeg", want: FileClassAudio},
		{mimeType: "video/mp4", want: FileClassVideo},
		{mimeType: "application/zip", want: FileClassArchive},
		{mimeType: "application/octet-stream", want: FileClassUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.mimeType, func(t *testing.T) {
			if got := ClassifyMIME(tc.mimeType); got != tc.want {
				t.Fatalf("ClassifyMIME(%q) = %q, want %q", tc.mimeType, got, tc.want)
			}
		})
	}
}

func TestRegistryPlanSelectsProcessorByFileClassAndCapability(t *testing.T) {
	registry := NewDefaultRegistry()
	cases := []struct {
		name          string
		mimeType      string
		wantClass     FileClass
		wantProcessor string
	}{
		{name: "text", mimeType: "text/plain", wantClass: FileClassText, wantProcessor: "polaris_text_extract"},
		{name: "pdf", mimeType: "application/pdf", wantClass: FileClassPDF, wantProcessor: "polaris_pdf_text_extract"},
		{name: "docx", mimeType: MIMEDOCX, wantClass: FileClassOfficeDocument, wantProcessor: "polaris_ooxml_text_extract"},
		{name: "xlsx", mimeType: MIMEXLSX, wantClass: FileClassOfficeSpreadsheet, wantProcessor: "polaris_ooxml_text_extract"},
		{name: "pptx", mimeType: MIMEPPTX, wantClass: FileClassOfficePresentation, wantProcessor: "polaris_ooxml_text_extract"},
		{name: "image", mimeType: "image/png", wantClass: FileClassImage, wantProcessor: "polaris_image_metadata"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := registry.Plan(Input{MimeType: tc.mimeType}, DefaultPlanOptionsForDerivedContext(tc.mimeType, ProfileFast))
			if err != nil {
				t.Fatalf("Plan(%s) error = %v", tc.mimeType, err)
			}
			if plan.Class != tc.wantClass {
				t.Fatalf("plan class = %q, want %q", plan.Class, tc.wantClass)
			}
			processor := plan.PrimaryProcessor()
			if processor == nil || processor.Name() != tc.wantProcessor {
				t.Fatalf("primary processor = %#v, want %s", processor, tc.wantProcessor)
			}
			descriptor := DescribeProcessor(processor)
			if descriptor.Name != tc.wantProcessor || len(descriptor.ArtifactKinds) == 0 || len(descriptor.Capabilities) == 0 {
				t.Fatalf("unexpected descriptor %#v", descriptor)
			}
		})
	}
}

func TestRegistryPlanRejectsUnsupportedCapabilities(t *testing.T) {
	registry := NewDefaultRegistry()
	_, err := registry.Plan(Input{MimeType: "image/png"}, PlanOptions{
		Profile:              ProfileFast,
		RequiredCapabilities: []ProcessorCapability{CapabilityImageCaption},
		PreferredArtifacts:   []ArtifactKind{ArtifactImageCaption},
	})
	if err == nil {
		t.Fatal("expected unsupported image caption plan")
	}
}
