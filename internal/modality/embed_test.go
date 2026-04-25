package modality

import (
	"encoding/json"
	"testing"
)

func TestEmbedInputJSONRoundTrip(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		var input EmbedInput
		if err := json.Unmarshal([]byte(`"hello"`), &input); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		values := input.Values()
		if len(values) != 1 || values[0] != "hello" {
			t.Fatalf("unexpected Values() = %#v", values)
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if string(encoded) != `"hello"` {
			t.Fatalf("unexpected Marshal() = %s", encoded)
		}
	})

	t.Run("array", func(t *testing.T) {
		var input EmbedInput
		if err := json.Unmarshal([]byte(`["one","two"]`), &input); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		values := input.Values()
		if len(values) != 2 || values[0] != "one" || values[1] != "two" {
			t.Fatalf("unexpected Values() = %#v", values)
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if string(encoded) != `["one","two"]` {
			t.Fatalf("unexpected Marshal() = %s", encoded)
		}
	})
}

func TestEmbeddingValuesJSONRoundTrip(t *testing.T) {
	t.Run("float array", func(t *testing.T) {
		value := EmbeddingValues{Float32: []float32{1.5, 2.25}}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if string(encoded) != `[1.5,2.25]` {
			t.Fatalf("unexpected Marshal() = %s", encoded)
		}

		var decoded EmbeddingValues
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if len(decoded.Float32) != 2 || decoded.Float32[0] != 1.5 || decoded.Float32[1] != 2.25 {
			t.Fatalf("unexpected decoded value = %#v", decoded)
		}
	})

	t.Run("base64", func(t *testing.T) {
		value := EmbeddingValues{Base64: "AQID"}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if string(encoded) != `"AQID"` {
			t.Fatalf("unexpected Marshal() = %s", encoded)
		}

		var decoded EmbeddingValues
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if decoded.Base64 != "AQID" {
			t.Fatalf("unexpected decoded value = %#v", decoded)
		}
	})
}
