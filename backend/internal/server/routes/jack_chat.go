package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// chatUploadBodyLimit covers the largest configurable attachment plus the
// multipart framing.
const chatUploadBodyLimit = 66 << 20

// registerJackChatRoutes mounts Jack's web chat mode. The group has no audit
// middleware: message bodies are conversation content, not panel changes.
func registerJackChatRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	if h.JackChat == nil {
		return
	}
	chat := v1.Group("/chat")
	chat.Use(gin.HandlerFunc(jwtAuth))
	chat.Use(middleware.BackendModeUserGuard(settingService))
	chat.Use(panelRateLimiter.Global())
	{
		chat.GET("/config", h.JackChat.Config)
		chat.GET("/groups/:id/models", h.JackChat.Models)
		chat.PUT("/preferences", h.JackChat.UpdatePreference)
		chat.GET("/conversations", h.JackChat.ListConversations)
		chat.POST("/conversations", h.JackChat.CreateConversation)
		chat.PATCH("/conversations/:id", h.JackChat.UpdateConversation)
		chat.DELETE("/conversations/:id", h.JackChat.DeleteConversation)
		chat.GET("/conversations/:id/messages", h.JackChat.Messages)
		chat.POST("/conversations/:id/messages", h.JackChat.SendMessage)
		chat.POST("/conversations/:id/images", h.JackChat.SendImages)
		chat.GET("/conversations/:id/stream", h.JackChat.StreamRun)
		chat.POST("/conversations/:id/stop", h.JackChat.StopRun)
		chat.POST("/attachments", middleware.RequestBodyLimit(chatUploadBodyLimit), h.JackChat.UploadAttachment)
		chat.DELETE("/attachments/:id", h.JackChat.DeleteAttachment)
		chat.GET("/attachments/:id/content", h.JackChat.AttachmentContent)
	}
}
