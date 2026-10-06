package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/jackchatkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogMarksChatModeAndMasksChatKey(t *testing.T) {
	chat := &service.UsageLog{APIKey: &service.APIKey{ID: 1, Key: jackchatkey.Prefix + "secretsecret", Name: jackchatkey.Name}}
	out := UsageLogFromService(chat)
	require.True(t, out.ChatMode)
	require.Equal(t, jackchatkey.Prefix+"***", out.APIKey.Key)
	require.True(t, UsageLogFromServiceAdmin(chat).ChatMode)

	normal := UsageLogFromService(&service.UsageLog{APIKey: &service.APIKey{ID: 2, Key: "sk-user"}})
	require.False(t, normal.ChatMode)
	require.Equal(t, "sk-user", normal.APIKey.Key)
}
