# Upstream v0.2.17 needs manual integration

Target commit: `060d0160417782cd962a0c7f460f6cd4565d4916`

Conflicting paths:

```
backend/cmd/server/wire_gen.go
```

Merge the target, remove this file, update upstream.json and rerun Jack checks.
