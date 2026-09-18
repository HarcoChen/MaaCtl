// Package event renders and prints MaaFramework tasker and context sink events.
package event

import (
	"fmt"
	"strings"
)

// DisplayChannels lists the focus display channels the protocol defines.
var DisplayChannels = []string{"log", "toast", "notification", "dialog", "modal"}

// ParseDisplay parses --focus-display: a comma-separated channel list or "all".
// An empty value keeps the default, which is "log" only.
func ParseDisplay(value string) (map[string]bool, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "log") {
		return nil, nil
	}
	allowed := map[string]bool{}
	if strings.EqualFold(value, "all") {
		for _, channel := range DisplayChannels {
			allowed[channel] = true
		}
		return allowed, nil
	}
	for _, part := range strings.Split(value, ",") {
		channel := strings.ToLower(strings.TrimSpace(part))
		if channel == "" {
			continue
		}
		known := false
		for _, candidate := range DisplayChannels {
			if candidate == channel {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("invalid --focus-display channel %q; use %s or all", channel, strings.Join(DisplayChannels, ","))
		}
		allowed[channel] = true
	}
	if len(allowed) == 0 {
		return nil, nil
	}
	return allowed, nil
}

// RenderFocus returns the message declared for an exact MaaFramework callback.
// A string focus is shorthand for {content: string, display: "log"}; an object
// is emitted only when one of its display channels is allowed. A nil filter
// means the CLI default, "log" only.
func RenderFocus(message string, detail any, allowed map[string]bool) string {
	values, ok := toObject(detail)
	if !ok {
		return ""
	}
	focus, ok := values["focus"].(map[string]any)
	if !ok {
		return ""
	}
	template, found := focus[message]
	if !found {
		return ""
	}
	content := ""
	switch v := template.(type) {
	case string:
		if !channelAllowed(allowed, "log") {
			return ""
		}
		content = v
	case map[string]any:
		if !displayAllowed(v["display"], allowed) {
			return ""
		}
		content, _ = v["content"].(string)
	}
	if content == "" {
		return ""
	}
	for key, value := range values {
		content = strings.ReplaceAll(content, "{"+key+"}", fmt.Sprint(value))
	}
	return content
}

func toObject(value any) (map[string]any, bool) {
	// Event detail structs are normalized through their JSON representation in the sink.
	if object, ok := value.(map[string]any); ok {
		return object, true
	}
	return nil, false
}

// channelAllowed reports whether a display channel is enabled. A nil filter
// means "log only", matching the CLI default.
func channelAllowed(allowed map[string]bool, channel string) bool {
	if allowed == nil {
		return channel == "log"
	}
	return allowed[channel]
}

// displayAllowed reports whether any of a template's display channels is
// enabled. A missing display means "log".
func displayAllowed(value any, allowed map[string]bool) bool {
	switch v := value.(type) {
	case nil:
		return channelAllowed(allowed, "log")
	case string:
		return channelAllowed(allowed, v)
	case []any:
		for _, item := range v {
			if text, ok := item.(string); ok && channelAllowed(allowed, text) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
