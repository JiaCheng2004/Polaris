//go:build !console

package console

import "net/http"

// Enabled reports whether this build embeds the console UI.
func Enabled() bool { return false }

// Handler returns the embedded-console HTTP handler. The default build embeds no
// UI, so it returns (nil, false); callers should fall back to running the console
// standalone (web/console) or using the console-tagged build.
func Handler() (http.Handler, bool) { return nil, false }
