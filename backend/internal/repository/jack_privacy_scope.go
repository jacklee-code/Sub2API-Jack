package repository

import "fmt"

// IN subqueries let PostgreSQL build provider ID sets instead of resolving
// account/group rows separately for every usage record in a model aggregate.
func openAIUsagePrivacyCondition(alias string) string {
	if alias == "" {
		alias = "usage_logs"
	}
	account, group := alias+".account_id", alias+".group_id"
	return fmt.Sprintf(`(%s NOT IN (SELECT id FROM accounts WHERE platform NOT IN ('', 'openai', 'composite')) AND (%s IN (SELECT id FROM accounts WHERE platform = 'openai') OR NOT COALESCE(%s IN (SELECT id FROM groups WHERE platform NOT IN ('', 'openai', 'composite')), FALSE)))`, account, account, group)
}

func openAIOpsPrivacyCondition(alias string) string {
	return fmt.Sprintf("COALESCE(%s.platform, '') IN ('', 'openai', 'composite')", alias)
}
