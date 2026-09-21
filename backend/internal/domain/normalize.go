package domain

import "strings"

func NormalizeCharacterName(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "'", "")
}
func Slug(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r == '\'' {
			return -1
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return r
	}, strings.ToLower(strings.TrimSpace(s)))), "-")
}
func GuildSlug(name, realm, region string) string { return Slug(region + "-" + realm + "-" + name) }
func RealmSlug(realm string) string               { return Slug(realm) }
