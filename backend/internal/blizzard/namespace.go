package blizzard

func DynamicNamespace(region string) string { return "dynamic-" + region }
func StaticNamespace(region string) string  { return "static-" + region }
func ProfileNamespace(region string) string { return "profile-" + region }
