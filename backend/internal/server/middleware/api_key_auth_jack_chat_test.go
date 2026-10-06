//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/jackchatkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthAcceptsChatKeysOnlyFromInternalDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 5, Name: "g", Status: service.StatusActive, Platform: service.PlatformOpenAI, Hydrated: true}
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10, Concurrency: 2}
	chatKey := &service.APIKey{ID: 9, UserID: user.ID, Key: jackchatkey.Prefix + "0123456789abcdef", Status: service.StatusActive, User: user, Group: group, GroupID: &group.ID}
	repo := &stubApiKeyRepo{getByKey: func(_ context.Context, key string) (*service.APIKey, error) {
		if key != chatKey.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		clone := *chatKey
		return &clone, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	keys := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	for name, mw := range map[string]gin.HandlerFunc{
		"openai": gin.HandlerFunc(NewAPIKeyAuthMiddleware(keys, nil, cfg)),
		"google": APIKeyAuthWithSubscriptionGoogle(keys, nil, cfg),
	} {
		router := gin.New()
		router.Use(mw)
		router.POST("/v1/responses", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		public := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		public.Header.Set("x-api-key", chatKey.Key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, public)
		require.Equal(t, http.StatusUnauthorized, w.Code, name)

		internal := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(jackchatkey.WithInternal(context.Background()))
		internal.Header.Set("x-api-key", chatKey.Key)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, internal)
		require.Equal(t, http.StatusNoContent, w.Code, name)
	}
}

func TestValidateCustomKeyRejectsChatPrefix(t *testing.T) {
	keys := service.NewAPIKeyService(&stubApiKeyRepo{}, nil, nil, nil, nil, nil, &config.Config{})
	require.Error(t, keys.ValidateCustomKey(jackchatkey.Prefix+"abcdefghijkl"))
	require.NoError(t, keys.ValidateCustomKey("sk-my-custom-key-123"))
}
