package understanding

import (
	"strings"
)

type FileClass string

const (
	FileClassText               FileClass = "text"
	FileClassImage              FileClass = "image"
	FileClassPDF                FileClass = "pdf"
	FileClassOfficeDocument     FileClass = "office_document"
	FileClassOfficeSpreadsheet  FileClass = "office_spreadsheet"
	FileClassOfficePresentation FileClass = "office_presentation"
	FileClassAudio              FileClass = "audio"
	FileClassVideo              FileClass = "video"
	FileClassArchive            FileClass = "archive"
	FileClassUnknown            FileClass = "unknown"
)

type ArtifactKind string

const (
	ArtifactText          ArtifactKind = "text"
	ArtifactMetadata      ArtifactKind = "metadata"
	ArtifactOCRText       ArtifactKind = "ocr_text"
	ArtifactLayout        ArtifactKind = "layout"
	ArtifactTable         ArtifactKind = "table"
	ArtifactForm          ArtifactKind = "form"
	ArtifactChunk         ArtifactKind = "chunk"
	ArtifactImageCaption  ArtifactKind = "image_caption"
	ArtifactImageMetadata ArtifactKind = "image_metadata"
	ArtifactEmbedding     ArtifactKind = "embedding"
	ArtifactRaw           ArtifactKind = "raw"
)

type ProcessorCapability string

const (
	CapabilityDetect       ProcessorCapability = "detect"
	CapabilityTextExtract  ProcessorCapability = "text_extract"
	CapabilityMetadata     ProcessorCapability = "metadata"
	CapabilityOCR          ProcessorCapability = "ocr"
	CapabilityLayout       ProcessorCapability = "layout"
	CapabilityTableExtract ProcessorCapability = "table_extract"
	CapabilityFormExtract  ProcessorCapability = "form_extract"
	CapabilityImageCaption ProcessorCapability = "image_caption"
	CapabilityChunk        ProcessorCapability = "chunk"
	CapabilityEmbed        ProcessorCapability = "embed"
)

type ProcessingProfile string

const (
	ProfileFast     ProcessingProfile = "fast"
	ProfileBalanced ProcessingProfile = "balanced"
	ProfileQuality  ProcessingProfile = "quality"
)

type LatencyTier string

const (
	LatencyLocalFast LatencyTier = "local_fast"
	LatencyLocalSlow LatencyTier = "local_slow"
	LatencyRemote    LatencyTier = "remote"
	LatencyAsync     LatencyTier = "async"
)

type CostTier string

const (
	CostFree   CostTier = "free"
	CostLow    CostTier = "low"
	CostMedium CostTier = "medium"
	CostHigh   CostTier = "high"
)

type ProcessorDescriptor struct {
	Name            string
	Version         string
	Backend         string
	Description     string
	FileClasses     []FileClass
	MIMETypes       []string
	ArtifactKinds   []ArtifactKind
	Capabilities    []ProcessorCapability
	Profiles        []ProcessingProfile
	LatencyTier     LatencyTier
	CostTier        CostTier
	Deterministic   bool
	Lossy           bool
	RequiresNetwork bool
	Priority        int
	Notes           []string
}

type DescribedProcessor interface {
	Descriptor() ProcessorDescriptor
}

type PlanOptions struct {
	Profile              ProcessingProfile
	RequiredCapabilities []ProcessorCapability
	PreferredArtifacts   []ArtifactKind
	AllowNetwork         bool
}

type PlanStep struct {
	Processor  Processor
	Descriptor ProcessorDescriptor
	Reason     string
}

type Plan struct {
	MimeType string
	Class    FileClass
	Profile  ProcessingProfile
	Steps    []PlanStep
	Warnings []string
}

func NormalizeProfile(profile string) ProcessingProfile {
	switch ProcessingProfile(strings.ToLower(strings.TrimSpace(profile))) {
	case ProfileFast:
		return ProfileFast
	case ProfileBalanced:
		return ProfileBalanced
	case ProfileQuality:
		return ProfileQuality
	default:
		return ProfileFast
	}
}

func ClassifyMIME(mimeType string) FileClass {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch {
	case strings.HasPrefix(mimeType, "text/"):
		return FileClassText
	case strings.HasPrefix(mimeType, "image/"):
		return FileClassImage
	case strings.HasPrefix(mimeType, "audio/"):
		return FileClassAudio
	case strings.HasPrefix(mimeType, "video/"):
		return FileClassVideo
	}
	switch mimeType {
	case "application/pdf":
		return FileClassPDF
	case "application/json", "application/xml", "application/x-yaml", "application/yaml", "application/javascript", "application/x-ndjson":
		return FileClassText
	case MIMEDOCX:
		return FileClassOfficeDocument
	case MIMEXLSX:
		return FileClassOfficeSpreadsheet
	case MIMEPPTX:
		return FileClassOfficePresentation
	case "application/zip", "application/x-zip-compressed", "application/x-tar", "application/gzip", "application/x-gzip":
		return FileClassArchive
	default:
		return FileClassUnknown
	}
}

func DefaultPlanOptionsForDerivedContext(mimeType string, profile ProcessingProfile) PlanOptions {
	capability := CapabilityTextExtract
	artifact := ArtifactText
	class := ClassifyMIME(mimeType)
	if class == FileClassImage {
		if profile == ProfileBalanced || profile == ProfileQuality {
			return PlanOptions{
				Profile:            profile,
				PreferredArtifacts: []ArtifactKind{ArtifactImageCaption, ArtifactOCRText, ArtifactImageMetadata, ArtifactMetadata},
				AllowNetwork:       true,
			}
		}
		capability = CapabilityMetadata
		artifact = ArtifactImageMetadata
	}
	if profile == ProfileBalanced || profile == ProfileQuality {
		switch class {
		case FileClassPDF, FileClassOfficeDocument, FileClassOfficeSpreadsheet, FileClassOfficePresentation:
			return PlanOptions{
				Profile:            profile,
				PreferredArtifacts: []ArtifactKind{ArtifactText, ArtifactOCRText, ArtifactLayout, ArtifactTable, ArtifactForm},
				AllowNetwork:       true,
			}
		}
	}
	return PlanOptions{
		Profile:              profile,
		RequiredCapabilities: []ProcessorCapability{capability},
		PreferredArtifacts:   []ArtifactKind{artifact},
	}
}

func (r *Registry) Plan(input Input, opts PlanOptions) (*Plan, error) {
	if r == nil {
		r = NewDefaultRegistry()
	}
	mimeType := CanonicalMIME(input.MimeType, input.Data)
	profile := opts.Profile
	if profile == "" {
		profile = ProfileFast
	}
	plan := &Plan{
		MimeType: mimeType,
		Class:    ClassifyMIME(mimeType),
		Profile:  profile,
	}
	for _, processor := range r.processors {
		if processor == nil || !processor.Supports(mimeType) {
			continue
		}
		descriptor := DescribeProcessor(processor)
		if descriptor.RequiresNetwork && !opts.AllowNetwork {
			continue
		}
		if !descriptorSupportsProfile(descriptor, profile) {
			continue
		}
		if !descriptorHasAllCapabilities(descriptor, opts.RequiredCapabilities) {
			continue
		}
		if len(opts.PreferredArtifacts) > 0 && !descriptorHasAnyArtifact(descriptor, opts.PreferredArtifacts) {
			continue
		}
		plan.Steps = append(plan.Steps, PlanStep{
			Processor:  processor,
			Descriptor: descriptor,
			Reason:     "selected_by_mime_profile_and_capability",
		})
	}
	if len(plan.Steps) == 0 {
		return nil, ErrUnsupportedMIME{MIMEType: mimeType}
	}
	sortPlanSteps(plan.Steps, profile)
	return plan, nil
}

func (p *Plan) PrimaryProcessor() Processor {
	if p == nil || len(p.Steps) == 0 {
		return nil
	}
	return p.Steps[0].Processor
}

func DescribeProcessor(processor Processor) ProcessorDescriptor {
	if processor == nil {
		return ProcessorDescriptor{}
	}
	if described, ok := processor.(DescribedProcessor); ok {
		descriptor := described.Descriptor()
		if descriptor.Name == "" {
			descriptor.Name = processor.Name()
		}
		if descriptor.Version == "" {
			descriptor.Version = processor.Version()
		}
		return descriptor
	}
	return ProcessorDescriptor{
		Name:          processor.Name(),
		Version:       processor.Version(),
		Backend:       "polaris_builtin",
		FileClasses:   []FileClass{FileClassUnknown},
		ArtifactKinds: []ArtifactKind{ArtifactText},
		Capabilities:  []ProcessorCapability{CapabilityTextExtract},
		Profiles:      []ProcessingProfile{ProfileFast, ProfileBalanced, ProfileQuality},
		LatencyTier:   LatencyLocalFast,
		CostTier:      CostFree,
		Deterministic: true,
		Lossy:         true,
	}
}

func (TextProcessor) Descriptor() ProcessorDescriptor {
	return ProcessorDescriptor{
		Name:          "polaris_text_extract",
		Version:       "v1",
		Backend:       "polaris_builtin",
		Description:   "Extracts UTF-8 text-like file bytes directly into derived context.",
		FileClasses:   []FileClass{FileClassText},
		MIMETypes:     []string{"text/*", "application/json", "application/xml", "application/x-yaml", "application/yaml", "application/javascript", "application/x-ndjson"},
		ArtifactKinds: []ArtifactKind{ArtifactText},
		Capabilities:  []ProcessorCapability{CapabilityTextExtract},
		Profiles:      []ProcessingProfile{ProfileFast, ProfileBalanced, ProfileQuality},
		LatencyTier:   LatencyLocalFast,
		CostTier:      CostFree,
		Deterministic: true,
		Lossy:         false,
	}
}

func (PDFTextProcessor) Descriptor() ProcessorDescriptor {
	return ProcessorDescriptor{
		Name:          "polaris_pdf_text_extract",
		Version:       "v1",
		Backend:       "polaris_builtin",
		Description:   "Best-effort text extraction from born-digital PDF content streams.",
		FileClasses:   []FileClass{FileClassPDF},
		MIMETypes:     []string{"application/pdf"},
		ArtifactKinds: []ArtifactKind{ArtifactText, ArtifactMetadata},
		Capabilities:  []ProcessorCapability{CapabilityTextExtract, CapabilityMetadata},
		Profiles:      []ProcessingProfile{ProfileFast, ProfileBalanced, ProfileQuality},
		LatencyTier:   LatencyLocalFast,
		CostTier:      CostFree,
		Deterministic: true,
		Lossy:         true,
		Notes:         []string{"Scanned PDFs, complex font encodings, tables, forms, and layout require OCR/layout processors."},
	}
}

func (OOXMLTextProcessor) Descriptor() ProcessorDescriptor {
	return ProcessorDescriptor{
		Name:          "polaris_ooxml_text_extract",
		Version:       "v1",
		Backend:       "polaris_builtin",
		Description:   "Extracts text from OOXML document, spreadsheet, and presentation packages.",
		FileClasses:   []FileClass{FileClassOfficeDocument, FileClassOfficeSpreadsheet, FileClassOfficePresentation},
		MIMETypes:     []string{MIMEDOCX, MIMEXLSX, MIMEPPTX},
		ArtifactKinds: []ArtifactKind{ArtifactText, ArtifactMetadata},
		Capabilities:  []ProcessorCapability{CapabilityTextExtract, CapabilityMetadata},
		Profiles:      []ProcessingProfile{ProfileFast, ProfileBalanced, ProfileQuality},
		LatencyTier:   LatencyLocalFast,
		CostTier:      CostFree,
		Deterministic: true,
		Lossy:         true,
		Notes:         []string{"Embedded images, charts, formulas, comments, tracked changes, and layout require specialized processors."},
	}
}

func (ImageMetadataProcessor) Descriptor() ProcessorDescriptor {
	return ProcessorDescriptor{
		Name:          "polaris_image_metadata",
		Version:       "v1",
		Backend:       "polaris_builtin",
		Description:   "Extracts deterministic image metadata such as format and dimensions.",
		FileClasses:   []FileClass{FileClassImage},
		MIMETypes:     []string{"image/*"},
		ArtifactKinds: []ArtifactKind{ArtifactImageMetadata, ArtifactMetadata},
		Capabilities:  []ProcessorCapability{CapabilityMetadata},
		Profiles:      []ProcessingProfile{ProfileFast, ProfileBalanced, ProfileQuality},
		LatencyTier:   LatencyLocalFast,
		CostTier:      CostFree,
		Deterministic: true,
		Lossy:         true,
		Notes:         []string{"Semantic image understanding requires OCR or image-caption processors."},
	}
}

func descriptorSupportsProfile(descriptor ProcessorDescriptor, profile ProcessingProfile) bool {
	if profile == "" || len(descriptor.Profiles) == 0 {
		return true
	}
	for _, existing := range descriptor.Profiles {
		if existing == profile {
			return true
		}
	}
	return false
}

func descriptorHasAllCapabilities(descriptor ProcessorDescriptor, required []ProcessorCapability) bool {
	for _, needed := range required {
		found := false
		for _, existing := range descriptor.Capabilities {
			if existing == needed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func descriptorHasAnyArtifact(descriptor ProcessorDescriptor, artifacts []ArtifactKind) bool {
	for _, wanted := range artifacts {
		for _, existing := range descriptor.ArtifactKinds {
			if existing == wanted {
				return true
			}
		}
	}
	return false
}

func sortPlanSteps(steps []PlanStep, profile ProcessingProfile) {
	if len(steps) < 2 {
		return
	}
	for i := 1; i < len(steps); i++ {
		step := steps[i]
		j := i - 1
		for j >= 0 && planStepRank(step, profile) > planStepRank(steps[j], profile) {
			steps[j+1] = steps[j]
			j--
		}
		steps[j+1] = step
	}
}

func planStepRank(step PlanStep, profile ProcessingProfile) int {
	rank := step.Descriptor.Priority
	if descriptorSupportsProfile(step.Descriptor, profile) {
		rank += 100
	}
	if step.Descriptor.RequiresNetwork {
		switch profile {
		case ProfileBalanced:
			rank += 10
		case ProfileQuality:
			rank += 25
		default:
			rank -= 20
		}
	}
	if !step.Descriptor.Lossy {
		rank += 5
	}
	return rank
}
