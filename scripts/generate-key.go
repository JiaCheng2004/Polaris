package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type output struct {
	Key    string `json:"key"`
	Hash   string `json:"hash"`
	Prefix string `json:"prefix"`
}

func main() {
	prefix := flag.String("prefix", "polaris_", "API key prefix")
	bytes := flag.Int("bytes", 32, "number of random bytes")
	plain := flag.Bool("plain", false, "print only the raw key")
	flag.Parse()

	if *bytes < 16 {
		exitf("bytes must be at least 16")
	}

	random := make([]byte, *bytes)
	if _, err := rand.Read(random); err != nil {
		exitf("generate random key: %v", err)
	}

	key := strings.TrimSpace(*prefix) + base64.RawURLEncoding.EncodeToString(random)
	sum := sha256.Sum256([]byte(key))
	result := output{
		Key:    key,
		Hash:   "sha256:" + hex.EncodeToString(sum[:]),
		Prefix: displayPrefix(key),
	}

	if *plain {
		fmt.Println(result.Key)
		return
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		exitf("encode output: %v", err)
	}
}

func displayPrefix(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12]
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
