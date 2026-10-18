package service

func modelIdentityLookupNames(_ string, model string) []string { return []string{model} }

// GroupAllowsModel preserves the configured model policy without provider-specific aliases.
func GroupAllowsModel(group *Group, model string) bool {
	return group == nil || !group.ModelAllowlistEnabled() || group.ModelAllowlist.Allows(model)
}
