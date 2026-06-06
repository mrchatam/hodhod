package i18n

// Admin returns a translated admin UI string.
func Admin(lang, key string) string {
	if lang == "" {
		lang = "fa"
	}
	m, ok := adminMessages[lang]
	if !ok {
		m = adminMessages["fa"]
	}
	s, ok := m[key]
	if !ok {
		if s, ok = adminMessages["en"][key]; ok {
			return s
		}
		return key
	}
	return s
}

var adminMessages = map[string]map[string]string{
	"fa": adminFA,
	"en": adminEN,
}
