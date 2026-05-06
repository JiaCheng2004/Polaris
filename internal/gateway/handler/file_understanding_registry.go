package handler

import (
	"fmt"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/understanding"
)

func fileUnderstandingRegistry(cfg config.FileUnderstandingConfig) (*understanding.Registry, error) {
	var processors []understanding.Processor
	for i, processorCfg := range cfg.Processors {
		if !processorCfg.Enabled {
			continue
		}
		backend := strings.TrimSpace(processorCfg.Backend)
		switch backend {
		case "tika":
			processor, err := understanding.NewTikaProcessor(understanding.TikaProcessorConfig{
				Name:      processorCfg.Name,
				Version:   processorCfg.Version,
				Endpoint:  processorCfg.Endpoint,
				Headers:   processorCfg.Headers,
				MIMETypes: processorCfg.MIMETypes,
				Profiles:  parseUnderstandingProfiles(processorCfg.Profiles),
				Timeout:   processorCfg.Timeout,
				Priority:  processorCfg.Priority,
				OCR:       processorCfg.OCR,
			})
			if err != nil {
				return nil, fmt.Errorf("files.understanding.processors[%d]: %w", i, err)
			}
			processors = append(processors, processor)
		case "http", "remote_http", "":
			processor, err := understanding.NewRemoteProcessor(understanding.RemoteProcessorConfig{
				Name:            processorCfg.Name,
				Version:         processorCfg.Version,
				Backend:         firstNonEmpty(processorCfg.Backend, "remote_http"),
				Endpoint:        processorCfg.Endpoint,
				Method:          processorCfg.Method,
				Headers:         processorCfg.Headers,
				MIMETypes:       processorCfg.MIMETypes,
				FileClasses:     parseUnderstandingFileClasses(processorCfg.FileClasses),
				ArtifactKinds:   parseUnderstandingArtifacts(processorCfg.Artifacts),
				Capabilities:    parseUnderstandingCapabilities(processorCfg.Capabilities),
				Profiles:        parseUnderstandingProfiles(processorCfg.Profiles),
				Timeout:         processorCfg.Timeout,
				Priority:        processorCfg.Priority,
				RequiresNetwork: true,
			})
			if err != nil {
				return nil, fmt.Errorf("files.understanding.processors[%d]: %w", i, err)
			}
			processors = append(processors, processor)
		default:
			return nil, fmt.Errorf("files.understanding.processors[%d]: unsupported backend %q", i, backend)
		}
	}
	return understanding.NewRegistryWithRemoteProcessors(processors...), nil
}

func parseUnderstandingProfiles(values []string) []understanding.ProcessingProfile {
	out := make([]understanding.ProcessingProfile, 0, len(values))
	for _, value := range values {
		profile := understanding.NormalizeProfile(value)
		if !containsProfile(out, profile) {
			out = append(out, profile)
		}
	}
	return out
}

func parseUnderstandingArtifacts(values []string) []understanding.ArtifactKind {
	out := make([]understanding.ArtifactKind, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, understanding.ArtifactKind(value))
		}
	}
	return out
}

func parseUnderstandingCapabilities(values []string) []understanding.ProcessorCapability {
	out := make([]understanding.ProcessorCapability, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, understanding.ProcessorCapability(value))
		}
	}
	return out
}

func parseUnderstandingFileClasses(values []string) []understanding.FileClass {
	out := make([]understanding.FileClass, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, understanding.FileClass(value))
		}
	}
	return out
}

func containsProfile(values []understanding.ProcessingProfile, target understanding.ProcessingProfile) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
