package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Translator resolves PI i18n strings (`$key`) through a language file declared
// in the top-level `languages` map.
type Translator struct {
	lang  string
	table map[string]string
}

// Lang returns the language code actually used ("" when the project declares no
// languages).
func (t *Translator) Lang() string {
	if t == nil {
		return ""
	}
	return t.lang
}

// Resolve returns the translation of `$key`. Plain strings pass through
// unchanged; an unknown reference loses its `$` so output stays readable.
func (t *Translator) Resolve(value string) string {
	if t == nil || t.table == nil {
		return strings.TrimPrefix(value, "$")
	}
	if !strings.HasPrefix(value, "$") {
		return value
	}
	key := strings.TrimPrefix(value, "$")
	if translated, ok := t.table[key]; ok {
		return translated
	}
	return key
}

// ResolveJSON walks a JSON-compatible value and resolves every i18n string in
// it. It is used for the PI_CONTROLLER / PI_RESOURCE environment variables,
// which the protocol requires to carry display-ready text.
func (t *Translator) ResolveJSON(value any) any {
	switch v := value.(type) {
	case string:
		return t.Resolve(v)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = t.ResolveJSON(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = t.ResolveJSON(item)
		}
		return out
	default:
		return value
	}
}

// ResolveLabels marshals v to JSON and resolves i18n strings in the result. It
// is the struct-friendly form of ResolveJSON.
func (t *Translator) ResolveLabels(v any) any {
	if t == nil || t.table == nil {
		return v
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return v
	}
	return t.ResolveJSON(generic)
}

// Translator returns the translator for lang, reading (and caching) the
// declared language file. A missing or unreadable file yields a translator that
// only strips `$` prefixes, so a broken translation never blocks a run.
func (l *Loaded) Translator(lang string) *Translator {
	if l == nil {
		return &Translator{}
	}
	if l.translators == nil {
		l.translators = map[string]*Translator{}
	}
	code := NegotiateLanguage(lang, l.Languages)
	if cached, ok := l.translators[code]; ok {
		return cached
	}
	translator := &Translator{lang: code}
	if fileName, ok := l.Languages[code]; ok && strings.TrimSpace(fileName) != "" {
		path := filepath.Join(l.Dir, filepath.FromSlash(strings.ReplaceAll(fileName, `\`, "/")))
		if b, err := os.ReadFile(path); err == nil {
			table := map[string]string{}
			if err := json.Unmarshal(StripJSONC(b), &table); err == nil {
				translator.table = table
			}
		}
	}
	l.translators[code] = translator
	return translator
}

// NegotiateLanguage picks the declared language code closest to requested.
// The matching order is: exact, case-insensitive, same primary subtag
// ("zh" ↔ "zh_cn"), then the conventional zh_cn / en_us / first-declared code.
func NegotiateLanguage(requested string, available map[string]string) string {
	if len(available) == 0 {
		return strings.TrimSpace(requested)
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return pickFallbackLanguage(available)
	}
	if _, ok := available[requested]; ok {
		return requested
	}
	lower := strings.ToLower(requested)
	for code := range available {
		if strings.ToLower(code) == lower {
			return code
		}
	}
	primary := primarySubtag(lower)
	for _, code := range sortedLanguages(available) {
		if primarySubtag(strings.ToLower(code)) == primary {
			return code
		}
	}
	return pickFallbackLanguage(available)
}

// pickFallbackLanguage prefers zh_cn, then en_us, then the first code in sorted
// order so the choice stays deterministic.
func pickFallbackLanguage(available map[string]string) string {
	for _, preferred := range []string{"zh_cn", "en_us"} {
		if _, ok := available[preferred]; ok {
			return preferred
		}
	}
	codes := sortedLanguages(available)
	if len(codes) == 0 {
		return ""
	}
	return codes[0]
}

// sortedLanguages returns the language codes in a stable order.
func sortedLanguages(available map[string]string) []string {
	codes := make([]string, 0, len(available))
	for code := range available {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// primarySubtag returns the part before the first separator, so "zh-cn",
// "zh_cn", and "zh" share the primary subtag "zh".
func primarySubtag(code string) string {
	if i := strings.IndexAny(code, "-_"); i >= 0 {
		return code[:i]
	}
	return code
}
