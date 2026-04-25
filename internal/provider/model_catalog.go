package provider

import (
	"sync"

	"github.com/JiaCheng2004/Polaris/internal/provider/catalog"
)

type ModelCatalogEntry = catalog.Entry
type ModelCatalogFamily = catalog.Family

func lookupModelCatalog(id string) (ModelCatalogEntry, bool) {
	cat, ok := providerCatalog()
	if !ok {
		return ModelCatalogEntry{}, false
	}
	entry, ok := cat.Lookup(id)
	return entry, ok
}

func lookupModelFamily(id string) (ModelCatalogFamily, bool) {
	cat, ok := providerCatalog()
	if !ok {
		return ModelCatalogFamily{}, false
	}
	family, ok := cat.Family(id)
	return family, ok
}

func providerCatalogProviders() []string {
	cat, ok := providerCatalog()
	if !ok {
		return nil
	}
	return cat.Providers()
}

var (
	providerCatalogOnce sync.Once
	providerCatalogRef  *catalog.Catalog
	providerCatalogErr  error
)

func prepareProviderCatalog() (*catalog.Catalog, error) {
	providerCatalogOnce.Do(func() {
		providerCatalogRef, providerCatalogErr = catalog.Default()
	})
	return providerCatalogRef, providerCatalogErr
}

func providerCatalog() (*catalog.Catalog, bool) {
	cat, err := prepareProviderCatalog()
	return cat, err == nil && cat != nil
}
