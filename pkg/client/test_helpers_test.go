package client

import "testing"

func newTestClient(t *testing.T, baseURL string, opts ...Option) *Client {
	t.Helper()

	client, err := New(baseURL, opts...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

func boolPtr(value bool) *bool {
	return &value
}
