package blob

import "testing"

func TestCleanS3KeyRejectsTraversal(t *testing.T) {
	for _, key := range []string{"", "../x", "a/../../b", "/absolute", `a\b`} {
		if _, err := cleanS3Key(key); err == nil {
			t.Fatalf("expected cleanS3Key(%q) to fail", key)
		}
	}
	clean, err := cleanS3Key("proj_123/pl_file_123")
	if err != nil {
		t.Fatalf("cleanS3Key() error = %v", err)
	}
	if clean != "proj_123/pl_file_123" {
		t.Fatalf("unexpected clean key %q", clean)
	}
}

func TestNormalizeS3Endpoint(t *testing.T) {
	endpoint, secure, err := normalizeS3Endpoint("https://minio.example.com", false)
	if err != nil {
		t.Fatalf("normalizeS3Endpoint() error = %v", err)
	}
	if endpoint != "minio.example.com" || !secure {
		t.Fatalf("unexpected endpoint=%q secure=%v", endpoint, secure)
	}
	if _, _, err := normalizeS3Endpoint("https://minio.example.com/path", true); err == nil {
		t.Fatal("expected endpoint with path to fail")
	}
}
