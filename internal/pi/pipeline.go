package pi

import (
	"strconv"
	"strings"
)

// Pipeline is a Pipeline override document: node name → node object.
type Pipeline = map[string]any

// maskedSecret is what display copies of a Pipeline put in place of a password
// value. It matches the mask used for Selection.MaskedValue.
const maskedSecret = "******"

// MergePipeline merges src into dst following MaaFramework's resource override
// semantics: node objects merge by node name, and within a node the same
// top-level field is replaced by src as a whole (arrays included). dst is
// mutated and returned so merges can be chained.
func MergePipeline(dst, src Pipeline) Pipeline {
	if dst == nil {
		dst = Pipeline{}
	}
	for node, value := range src {
		srcNode, ok := asObject(value)
		if !ok {
			dst[node] = CloneJSON(value)
			continue
		}
		dstNode, ok := asObject(dst[node])
		if !ok {
			dstNode = map[string]any{}
		}
		for field, fieldValue := range srcNode {
			dstNode[field] = CloneJSON(fieldValue)
		}
		dst[node] = dstNode
	}
	return dst
}

// CloneJSON deep-copies a JSON-compatible value so merged documents never share
// substructure with their sources.
func CloneJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = CloneJSON(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = CloneJSON(item)
		}
		return out
	default:
		return v
	}
}

// maskSecrets deep-copies a JSON-compatible value and replaces every string
// that contains one of the secret literals. A matching string is replaced as a
// whole because a partial mask could still leak the secret through the rest of
// the text; non-string leaves are left alone.
func maskSecrets(value any, secrets []string) any {
	switch v := value.(type) {
	case string:
		for _, secret := range secrets {
			if secret != "" && strings.Contains(v, secret) {
				return maskedSecret
			}
		}
		return v
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = maskSecrets(item, secrets)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = maskSecrets(item, secrets)
		}
		return out
	default:
		return v
	}
}

// maskedPipeline returns a display copy of a Pipeline with the secrets masked.
// A nil Pipeline stays nil; an empty one stays an empty (non-nil) map so the
// JSON `omitempty` behavior of callers is unchanged.
func maskedPipeline(pipeline Pipeline, secrets []string) Pipeline {
	if pipeline == nil {
		return nil
	}
	masked, _ := maskSecrets(pipeline, secrets).(map[string]any)
	return masked
}

// asObject narrows a JSON value to an object.
func asObject(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

// placeholder is one resolved template variable. text is the rendered form;
// typed is the raw value used when a string is exactly the placeholder.
type placeholder struct {
	text  string
	typed any
}

// substituteTemplates walks a Pipeline override template and replaces
// `{name}` placeholders in strings. A string that consists solely of a
// placeholder becomes the placeholder's typed value, which is how input
// options keep `int`/`bool` types in the resulting Pipeline.
func substituteTemplates(value any, values map[string]placeholder) (any, error) {
	switch v := value.(type) {
	case string:
		return substituteString(v, values), nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			substituted, err := substituteTemplates(item, values)
			if err != nil {
				return nil, err
			}
			out[key] = substituted
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			substituted, err := substituteTemplates(item, values)
			if err != nil {
				return nil, err
			}
			out[i] = substituted
		}
		return out, nil
	default:
		return v, nil
	}
}

// substituteString replaces every `{name}` occurrence. Whole-string matches
// return the typed value directly.
func substituteString(text string, values map[string]placeholder) any {
	if len(text) >= 2 && text[0] == '{' && text[len(text)-1] == '}' {
		if value, ok := values[text[1:len(text)-1]]; ok {
			return value.typed
		}
	}
	// Fast path: a string without any placeholder is returned unchanged.
	if !hasPlaceholder(text) {
		return text
	}
	var out []byte
	for i := 0; i < len(text); {
		if text[i] == '{' {
			if end := indexByteFrom(text, '}', i+1); end >= 0 {
				if value, ok := values[text[i+1:end]]; ok {
					out = append(out, value.text...)
					i = end + 1
					continue
				}
			}
		}
		out = append(out, text[i])
		i++
	}
	return string(out)
}

func hasPlaceholder(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] == '{' {
			return true
		}
	}
	return false
}

func indexByteFrom(text string, target byte, from int) int {
	for i := from; i < len(text); i++ {
		if text[i] == target {
			return i
		}
	}
	return -1
}

// jsonNumber converts an input field's textual value into the JSON number that
// best represents it, so `pipeline_type: int` values stay integral.
func jsonNumber(value string) any {
	if asInt, err := strconv.ParseInt(value, 10, 64); err == nil {
		return asInt
	}
	if asFloat, err := strconv.ParseFloat(value, 64); err == nil {
		return asFloat
	}
	return value
}
