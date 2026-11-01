package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestJackPrivacyErrorHistoryPreservesUsefulDiagnostics(t *testing.T) {
	record := &OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{Platform: PlatformOpenAI, Model: "private-model", RequestedModel: "public-model", Message: "private-model rejected account_id=acct-secret; retry in 12 seconds"},
		ErrorBody:   `{"error":{"message":"private-model unavailable for acct-secret; retry in 12 seconds","account_id":"acct-secret","code":"custom_code","param":"budget_tokens","budget_tokens":1024}}`,
	}
	raw := record.ErrorBody
	public := ToUserErrorRequestDetail(record)
	require.Equal(t, "[redacted] rejected account_id=[redacted]; retry in 12 seconds", public.Message)
	require.Contains(t, public.ErrorBody, "[redacted] unavailable for [redacted]; retry in 12 seconds")
	require.Contains(t, public.ErrorBody, `"budget_tokens":1024`)
	require.Contains(t, public.ErrorBody, `"code":"custom_code"`)
	require.Equal(t, raw, record.ErrorBody, "admin source evidence must remain intact")
	record.Platform = PlatformAnthropic
	other := ToUserErrorRequestDetail(record)
	require.Equal(t, raw, other.ErrorBody)
	require.Equal(t, record.Message, other.Message)
}
