# Upstream v0.2.20 needs manual integration

Target commit: `dab829a4e9b7d1cc307189aea8b0319eb29e29d0`

Conflicting paths:

```
backend/cmd/server/wire.go
backend/cmd/server/wire_gen.go
backend/internal/handler/admin/openai_oauth_handler.go
backend/internal/pkg/httputil/body.go
backend/internal/repository/http_upstream.go
backend/internal/server/middleware/api_key_auth.go
backend/internal/service/openai_account_scheduler.go
backend/internal/service/openai_quota_service.go
frontend/pnpm-lock.yaml
```

Merge the target, remove this file, update upstream.json and rerun Jack checks.
