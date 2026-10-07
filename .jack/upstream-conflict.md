# Upstream v0.2.14 needs manual integration

Target commit: `0363b8cdba8cec3e2ba4b2dbd49c4481143fa55d`

Conflicting paths:

```
.github/audit-exceptions.yml
frontend/pnpm-lock.yaml
```

Merge the target, remove this file, update upstream.json and rerun Jack checks.
