package bytedance

import "testing"

func TestResolveBytedanceModelAlias(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "chat current flagship",
			input: "doubao-seed-2.0-pro",
			want:  "doubao-seed-2-0-pro-260215",
		},
		{
			name:  "vision current",
			input: "doubao-seed-1.6-vision",
			want:  "doubao-seed-1-6-vision-250815",
		},
		{
			name:  "image current",
			input: "doubao-seedream-5.0-lite",
			want:  "doubao-seedream-5-0-lite-260128",
		},
		{
			name:  "video current",
			input: "doubao-seedance-2.0",
			want:  "doubao-seedance-2-0-260128",
		},
		{
			name:  "legacy video alias remains supported",
			input: "seedance-2.0",
			want:  "doubao-seedance-2-0-260128",
		},
		{
			name:  "video fast current",
			input: "doubao-seedance-2.0-fast",
			want:  "doubao-seedance-2-0-fast-260128",
		},
		{
			name:  "legacy video fast alias remains supported",
			input: "seedance-2.0-fast",
			want:  "doubao-seedance-2-0-fast-260128",
		},
		{
			name:  "unknown model passthrough",
			input: "doubao-pro-256k",
			want:  "doubao-pro-256k",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveBytedanceModelAlias(tc.input); got != tc.want {
				t.Fatalf("resolveBytedanceModelAlias(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
