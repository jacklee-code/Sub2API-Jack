# Upstream v0.2.19 needs manual integration

Target commit: `dece26153621959ccd7635814f4b2d08304e681a`

Conflicting paths:

```
backend/go.mod
backend/go.sum
backend/internal/handler/admin/subscription_handler.go
frontend/src/i18n/locales/en/index.ts
frontend/src/i18n/locales/zh/index.ts
```

Merge the target, remove this file, update upstream.json and rerun Jack checks.
