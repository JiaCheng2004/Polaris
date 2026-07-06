package provider

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
)

type providerRegistrar func(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig)

var (
	providerFamilyRegistrars         = map[string]providerRegistrar{}
	providerFamilyRegistrationErrors []error
)

func registerProviderFamilyRegistrar(name string, registrar providerRegistrar) {
	if err := addProviderFamilyRegistrar(providerFamilyRegistrars, name, registrar); err != nil {
		providerFamilyRegistrationErrors = append(providerFamilyRegistrationErrors, err)
	}
}

func addProviderFamilyRegistrar(registrars map[string]providerRegistrar, name string, registrar providerRegistrar) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("provider family name is required")
	}
	if registrar == nil {
		return fmt.Errorf("provider family %s registrar is required", name)
	}
	if _, exists := registrars[name]; exists {
		return fmt.Errorf("provider family %s already registered", name)
	}
	registrars[name] = registrar
	return nil
}

func supportedProviderFamilies() []string {
	names := make([]string, 0, len(providerFamilyRegistrars))
	for name := range providerFamilyRegistrars {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func providerFamilyRegistrationError() error {
	if len(providerFamilyRegistrationErrors) == 0 {
		return nil
	}
	return errors.Join(providerFamilyRegistrationErrors...)
}

func registerProviderFamily(registry *Registry, warnings *[]string, providerName string, providerCfg config.ProviderConfig) bool {
	registrar, ok := providerFamilyRegistrars[providerName]
	if !ok {
		return false
	}
	registrar(registry, warnings, providerName, providerCfg)
	return true
}

func ProviderRuntimeEnabled(name string, providerCfg config.ProviderConfig) bool {
	switch name {
	case "bedrock":
		return strings.TrimSpace(providerCfg.AccessKeyID) != "" &&
			strings.TrimSpace(providerCfg.AccessKeySecret) != "" &&
			strings.TrimSpace(providerCfg.Location) != ""
	case "bytedance":
		return strings.TrimSpace(providerCfg.APIKey) != "" ||
			(strings.TrimSpace(providerCfg.AccessKeyID) != "" && strings.TrimSpace(providerCfg.AccessKeySecret) != "") ||
			strings.TrimSpace(providerCfg.SpeechAPIKey) != "" ||
			(strings.TrimSpace(providerCfg.AppID) != "" && strings.TrimSpace(providerCfg.SpeechAccessToken) != "")
	case "google-vertex":
		return strings.TrimSpace(providerCfg.ProjectID) != "" && strings.TrimSpace(providerCfg.Location) != "" && strings.TrimSpace(providerCfg.SecretKey) != ""
	case "ollama":
		return true
	default:
		return strings.TrimSpace(providerCfg.APIKey) != ""
	}
}

func providerEnabled(name string, providerCfg config.ProviderConfig) bool {
	return ProviderRuntimeEnabled(name, providerCfg)
}
