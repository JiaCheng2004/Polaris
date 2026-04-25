package catalog

import "testing"

func TestDefaultCatalogLoadsAndValidates(t *testing.T) {
	t.Parallel()

	cat, err := Default()
	if err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if len(cat.entriesByID) == 0 {
		t.Fatal("expected embedded catalog entries")
	}
	if _, ok := cat.Lookup("openai/gpt-5.4"); !ok {
		t.Fatal("expected openai/gpt-5.4 in embedded catalog")
	}
	if _, ok := cat.Lookup("openai/gpt-5.5"); !ok {
		t.Fatal("expected openai/gpt-5.5 in embedded catalog")
	}
	if _, ok := cat.Lookup("openai/gpt-image-2"); !ok {
		t.Fatal("expected openai/gpt-image-2 in embedded catalog")
	}
	if target, ok := cat.AliasTarget("GLM-5.1"); !ok || target != "glm/glm-5.1" {
		t.Fatalf("alias target = %q, %v", target, ok)
	}
	if family, ok := cat.Family("gpt-5.4"); !ok || family.DisplayName != "GPT-5.4" {
		t.Fatalf("family = %#v, %v", family, ok)
	}
}

func TestParseCatalogRejectsAliasCollision(t *testing.T) {
	t.Parallel()

	_, err := parseCatalog([]byte(`
models:
  - provider: one
    model_id: alpha
    display_name: Alpha
    aliases: ["Shared Alias"]
    family_id: alpha-family
    family_display_name: Alpha Family
    family_aliases: ["Alpha Family"]
    family_priority: 0
    modality: chat
    capabilities: [streaming]
    status: ga
    verification_class: strict
    doc_url: https://example.com/alpha
    last_verified: "2026-04-22"
  - provider: two
    model_id: beta
    display_name: Beta
    aliases: ["Shared Alias"]
    family_id: beta-family
    family_display_name: Beta Family
    family_aliases: ["Beta Family"]
    family_priority: 0
    modality: chat
    capabilities: [streaming]
    status: ga
    verification_class: strict
    doc_url: https://example.com/beta
    last_verified: "2026-04-22"
`))
	if err == nil {
		t.Fatal("expected alias collision error")
	}
}

func TestParseCatalogRejectsMissingMetadata(t *testing.T) {
	t.Parallel()

	_, err := parseCatalog([]byte(`
models:
  - provider: openai
    model_id: broken
    display_name: Broken
    aliases: []
    family_id: broken
    family_display_name: Broken
    family_aliases: []
    family_priority: 0
    modality: chat
    capabilities: [streaming]
    status: ga
    verification_class: strict
    doc_url: ""
    last_verified: "2026-04-22"
`))
	if err == nil {
		t.Fatal("expected missing metadata error")
	}
}

func TestParseCatalogRejectsFamilyAliasCollision(t *testing.T) {
	t.Parallel()

	_, err := parseCatalog([]byte(`
models:
  - provider: one
    model_id: alpha
    display_name: Alpha
    aliases: []
    family_id: shared
    family_display_name: Shared
    family_aliases: ["Shared Family"]
    family_priority: 0
    modality: chat
    capabilities: [streaming]
    status: ga
    verification_class: strict
    doc_url: https://example.com/alpha
    last_verified: "2026-04-22"
  - provider: two
    model_id: beta
    display_name: Beta
    aliases: []
    family_id: second
    family_display_name: Second
    family_aliases: ["Shared Family"]
    family_priority: 0
    modality: chat
    capabilities: [streaming]
    status: ga
    verification_class: strict
    doc_url: https://example.com/beta
    last_verified: "2026-04-22"
`))
	if err == nil {
		t.Fatal("expected family alias collision error")
	}
}
