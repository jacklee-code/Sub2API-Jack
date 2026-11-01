package jackprivacy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaAndCloseFrames(t *testing.T) {
	p := New(true, false, "application/json", "public-model", nil)
	ctx := WithPolicy(context.Background(), p)
	audio := []byte{0, 1, 0xff, 3}
	require.Equal(t, audio, ClientMediaFrame(ctx, audio))
	require.NotContains(t, string(ClientMediaFrame(ctx, []byte(`{"model":"private-model","account_id":"provider-id"}`))), "private-model")
	require.Equal(t, "Request ended", CloseReason(ctx, "private-model for provider-id"))
	require.Empty(t, CloseReason(ctx, ""))
	p.SetAdmin(true)
	require.Equal(t, "private-model for provider-id", CloseReason(ctx, "private-model for provider-id"))
}
