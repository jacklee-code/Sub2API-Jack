// Package jackchatkey identifies the hidden API keys that Jack's chat mode
// creates for each (user, group) pair. The prefix keeps the check free of
// database lookups so the key list, usage DTOs and the public gateway can all
// recognise chat keys cheaply.
package jackchatkey

import (
	"context"
	"strings"
)

// Prefix starts every chat-mode key. Users cannot create custom keys with it.
const Prefix = "sk-jackchat-"

// Name is the stored name of chat-mode keys.
const Name = "Chat Mode"

// IsChatKey reports whether key belongs to chat mode.
func IsChatKey(key string) bool {
	return strings.HasPrefix(key, Prefix)
}

type internalMarker struct{}

// WithInternal marks a request context as an in-process chat-mode dispatch.
// Only code in this process can set it; an incoming HTTP request never has it.
func WithInternal(ctx context.Context) context.Context {
	return context.WithValue(ctx, internalMarker{}, true)
}

// IsInternal reports whether ctx came from an in-process chat-mode dispatch.
func IsInternal(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(internalMarker{}).(bool)
	return v
}
