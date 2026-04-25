package provider

import (
	"slices"
	"testing"
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
