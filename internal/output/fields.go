package output

import (
	"encoding/json"
	"strings"
)

// Filter returns a new value containing only the requested paths. Paths use
// dotted notation (e.g. "data.id"); when a path segment encounters an array,
// the remaining segments are applied to each element ("array broadcast").
//
// If paths is empty, the original value is returned unchanged. Missing paths
// are silently skipped — callers do not need to know which fields a given
// response contains.
//
// json.RawMessage inputs are unmarshaled into a generic value before filtering.
func Filter(value any, paths []string) any {
	if len(paths) == 0 {
		return value
	}

	// Normalize json.RawMessage / []byte into a generic value.
	switch v := value.(type) {
	case json.RawMessage:
		var decoded any
		if err := json.Unmarshal(v, &decoded); err != nil {
			return value
		}
		return filter(decoded, paths)
	case []byte:
		var decoded any
		if err := json.Unmarshal(v, &decoded); err != nil {
			return value
		}
		return filter(decoded, paths)
	}

	return filter(value, paths)
}

// filter walks the generic value tree, retaining only the nodes named by the
// provided paths.
func filter(value any, paths []string) any {
	groups, hasTerminal := groupPaths(paths)

	switch v := value.(type) {
	case map[string]any:
		if hasTerminal {
			// A bare path (e.g. "data" with nothing after) means "keep this
			// whole subtree" — but we may also have deeper paths under the
			// same key (e.g. "data" and "data.id"). Treat the bare path as
			// dominant: keep the whole subtree without further filtering.
			return v
		}

		// Smart wrapper fallback: list-style endpoints commonly return
		// {"data": [...], ...}. If none of the requested paths match a
		// top-level key but there is exactly one array-valued sibling,
		// rewrite paths to broadcast through that wrapper. Mixed cases
		// (some paths match, some don't) keep strict semantics.
		anyMatch := false
		for key := range groups {
			if _, ok := v[key]; ok {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			arrayKeys := []string{}
			for key, val := range v {
				if isJSONArray(val) {
					arrayKeys = append(arrayKeys, key)
				}
			}
			if len(arrayKeys) == 1 {
				wrapper := arrayKeys[0]
				rewritten := make([]string, len(paths))
				for i, p := range paths {
					rewritten[i] = wrapper + "." + p
				}
				return filter(v, rewritten)
			}
		}

		out := make(map[string]any, len(groups))
		for key, remaining := range groups {
			child, ok := v[key]
			if !ok {
				continue
			}
			out[key] = filter(child, remaining)
		}
		return out
	case []any:
		// Array broadcast: apply the same paths to every element.
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = filter(item, paths)
		}
		return out
	case []json.RawMessage:
		// Array of unparsed JSON values — operations that page through API
		// responses without fully decoding each row hand back this shape
		// (e.g. workspaces list). Decode each element on the fly so the
		// filter logic doesn't need to know about late-bound JSON.
		out := make([]any, len(v))
		for i, item := range v {
			var decoded any
			if err := json.Unmarshal(item, &decoded); err != nil {
				out[i] = item
				continue
			}
			out[i] = filter(decoded, paths)
		}
		return out
	default:
		// Primitive — nothing to filter.
		return v
	}
}

// isJSONArray reports whether v is one of the array shapes the filter knows
// how to broadcast through.
func isJSONArray(v any) bool {
	switch v.(type) {
	case []any, []json.RawMessage:
		return true
	}
	return false
}

// groupPaths splits each path on its first "." and returns a map of head
// segment to remaining paths. Bare paths (no dot) produce an empty remaining
// path, which signals "keep this subtree as-is" via hasTerminal.
func groupPaths(paths []string) (map[string][]string, bool) {
	groups := map[string][]string{}
	hasTerminal := false
	for _, p := range paths {
		if p == "" {
			hasTerminal = true
			continue
		}
		head, tail, hasTail := strings.Cut(p, ".")
		if !hasTail {
			groups[head] = append(groups[head], "")
			continue
		}
		groups[head] = append(groups[head], tail)
	}
	return groups, hasTerminal
}
