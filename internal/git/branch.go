package git

import "strings"

// maxSlugLen caps the slug portion of a branch name.
const maxSlugLen = 50

// BranchName builds a branch name from a pattern containing {key} and {slug}.
// The key is lowercased and the title is slugified.
func BranchName(pattern, key, title string) string {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	slug := Slugify(title)
	// A punctuation/CJK-only title slugifies to "". Don't emit a branch that
	// loses the issue identity (which would make branch reuse collide across
	// issues): when the pattern carries no {key}, fill the {slug} slot with the
	// key so the result stays unique; otherwise just drop the empty slug.
	if slug == "" && !strings.Contains(pattern, "{key}") {
		slug = lowerKey
	}
	name := strings.ReplaceAll(pattern, "{key}", lowerKey)
	name = strings.ReplaceAll(name, "{slug}", slug)
	// Trim separators left dangling by an empty slug (e.g. "{key}-{slug}" ->
	// "demo-1-" or "feature/{slug}" -> "feature/"), which git rejects as a ref.
	name = strings.Trim(name, "-/")
	if name == "" {
		return lowerKey
	}
	return name
}

// accents maps common Latin diacritics to their ASCII base letter so that
// Portuguese/Spanish titles produce clean slugs.
var accents = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

// Slugify converts a title into a lowercase, dash-separated ASCII slug.
func Slugify(s string) string {
	var b strings.Builder
	lastDash := true // avoids a leading dash
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if base, ok := accents[r]; ok {
			r = base
		}
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
		if b.Len() >= maxSlugLen {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
