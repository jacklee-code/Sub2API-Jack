# Upstream v0.2.21 needs manual integration

Target commit: `10ba8f9b4dc2e6023d4a9879ab77b21841babd10`

Conflicting paths:

```
backend/internal/repository/usage_log_repo_insert.go
backend/internal/repository/usage_log_repo_insert_shape_unit_test.go
backend/internal/repository/usage_log_repo_query.go
backend/internal/repository/usage_log_repo_request_type_test.go
backend/internal/repository/usage_log_session_id_unit_test.go
backend/internal/service/openai_responses_orphan_precheck_test.go
```

Merge the target, remove this file, update upstream.json and rerun Jack checks.
