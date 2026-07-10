// Package console optionally serves the Polaris admin console (web/console) from
// the gateway binary. It is compiled out by default: the standard build carries
// no UI, stays pure-Go, and needs no Node build. Building with the `console` tag
// (see `make build-console`) embeds the built SPA and enables the --console flag,
// which serves it on its own listener without touching the API's routes.
package console
