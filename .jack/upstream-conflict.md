# Upstream v0.2.16 needs manual integration

Target commit: `01842576b3d5c1e2545b5a99ad555d36e28ed306`

Conflicting paths:

```
backend/cmd/server/wire_gen.go
```

Merge the target, remove this file, update upstream.json and rerun Jack checks.
