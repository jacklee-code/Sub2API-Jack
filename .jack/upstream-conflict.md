# Upstream v0.2.15 needs manual integration

Target commit: `f2669c8cf62555cd92389b3f55920e9e6e7c6ff2`

Conflicting paths:

```
backend/go.mod
frontend/src/api/__tests__/settings.authSourceDefaults.spec.ts
frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
```

Merge the target, remove this file, update upstream.json and rerun Jack checks.
