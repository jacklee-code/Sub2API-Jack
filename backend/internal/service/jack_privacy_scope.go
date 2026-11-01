package service

// RoutingPrivacyPlatform resolves stored usage by the actual account first.
// A composite group alone cannot identify which provider served a request.
func (l *UsageLog) RoutingPrivacyPlatform() string {
	if l == nil {
		return ""
	}
	if l.Account != nil && l.Account.Platform != "" {
		return l.Account.Platform
	}
	if l.RoutingProvider != "" {
		return l.RoutingProvider
	}
	if l.Group != nil && l.Group.Platform != "" {
		return l.Group.Platform
	}
	return ""
}

// Historical rows with no provider evidence stay protected. Known providers
// other than OpenAI retain their pre-privacy display and query behavior.
func ProtectOpenAIRouting(platform string) bool {
	return platform == PlatformOpenAI || platform == "" || platform == PlatformComposite
}
