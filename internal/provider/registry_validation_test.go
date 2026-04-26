package provider

import (
	"slices"
	"strings"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/config"
)

func TestCatalogProvidersAreSupportedByRegistryFactories(t *testing.T) {
	t.Parallel()

	supported := supportedProviderFamilies()
	for _, providerName := range providerCatalogProviders() {
		if !slices.Contains(supported, providerName) {
			t.Fatalf("catalog provider %q is not covered by registry factories", providerName)
		}
	}
}

func TestProviderFamilyRegistrarRejectsDuplicates(t *testing.T) {
	t.Parallel()

	registrars := map[string]providerRegistrar{}
	registrar := func(*Registry, *[]string, string, config.ProviderConfig) {}
	if err := addProviderFamilyRegistrar(registrars, "example", registrar); err != nil {
		t.Fatalf("addProviderFamilyRegistrar(first) error = %v", err)
	}
	err := addProviderFamilyRegistrar(registrars, "example", registrar)
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate registration error, got %v", err)
	}
}
