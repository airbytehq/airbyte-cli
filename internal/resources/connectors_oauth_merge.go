package resources

import (
	"time"
)

// mergeOAuthCredentials merges the OAuth auth_payload into a connector's
// replication config, using the template's user_config_spec.advanced_auth
// declarations to figure out where each credential field belongs. This is a
// faithful port of the agent-engine-mcp connector_flow.merge_oauth_credentials
// implementation.
//
// Algorithm:
//  1. Start from partialConfig (a copy).
//  2. Apply spec defaults from connectionSpecification (required fields with
//     defaults; dynamic start_date for known patterns).
//  3. If advanced_auth declares a predicate_key + predicate_value, set them.
//  4. For each property in complete_oauth_server/output_specification, if the
//     auth payload has a matching key, place it at the path declared by
//     path_in_connector_config.
//  5. If no oauth output spec exists, fall back to a flat merge of the auth
//     payload into the config.
func mergeOAuthCredentials(userConfigSpec map[string]any, partialConfig map[string]any, authPayload map[string]any) map[string]any {
	// Copy partialConfig so the caller's map isn't mutated.
	sourceConfig := make(map[string]any, len(partialConfig))
	for k, v := range partialConfig {
		sourceConfig[k] = v
	}

	applySpecDefaults(sourceConfig, userConfigSpec)

	advancedAuth, _ := userConfigSpec["advanced_auth"].(map[string]any)

	if advancedAuth != nil {
		predicateKey, _ := advancedAuth["predicate_key"].([]any)
		predicateValue := advancedAuth["predicate_value"]
		if len(predicateKey) > 0 && predicateValue != nil {
			path := stringSliceFromAny(predicateKey)
			if len(path) > 0 {
				setNestedValue(sourceConfig, path, predicateValue)
			}
		}

		oauthSpec, _ := advancedAuth["oauth_config_specification"].(map[string]any)
		if oauthSpec != nil {
			serverOutput, _ := oauthSpec["complete_oauth_server_output_specification"].(map[string]any)
			clientOutput, _ := oauthSpec["complete_oauth_output_specification"].(map[string]any)

			allProperties := map[string]any{}
			collectProperties(allProperties, serverOutput)
			collectProperties(allProperties, clientOutput)

			if len(allProperties) > 0 {
				for propName, propSpecAny := range allProperties {
					propSpec, ok := propSpecAny.(map[string]any)
					if !ok {
						continue
					}
					rawPath, ok := propSpec["path_in_connector_config"].([]any)
					if !ok {
						continue
					}
					path := stringSliceFromAny(rawPath)
					if len(path) == 0 {
						continue
					}
					authValue, has := authPayload[propName]
					if !has {
						continue
					}
					setNestedValue(sourceConfig, path, authValue)
				}
				return sourceConfig
			}
		}
	}

	// Fall-through: no oauth spec found. Flat-merge the auth payload.
	for k, v := range authPayload {
		sourceConfig[k] = v
	}
	return sourceConfig
}

func collectProperties(into map[string]any, from map[string]any) {
	if from == nil {
		return
	}
	props, ok := from["properties"].(map[string]any)
	if !ok {
		return
	}
	for k, v := range props {
		into[k] = v
	}
}

// setNestedValue sets value at the given path of dot-separated keys, creating
// intermediate maps as needed.
func setNestedValue(obj map[string]any, path []string, value any) {
	for i, key := range path {
		if i == len(path)-1 {
			obj[key] = value
			return
		}
		next, ok := obj[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			obj[key] = next
		}
		obj = next
	}
}

func stringSliceFromAny(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		s, ok := v.(string)
		if !ok {
			return nil // path with non-string segment is invalid; skip the whole path
		}
		out = append(out, s)
	}
	return out
}

// dynamicDateFieldPatterns are required fields whose value the MCP fills in
// with a default of "2 years ago" when no explicit default is declared. These
// are mostly start_date-style filters for incremental syncs.
var dynamicDateFieldPatterns = map[string]bool{
	"start_date":     true,
	"startDate":      true,
	"from_date":      true,
	"since_date":     true,
	"sync_from_date": true,
}

// applySpecDefaults fills in required fields with defaults declared in
// connectionSpecification. For dynamic-date fields without an explicit
// default, picks "2 years ago" formatted to match the field's pattern.
func applySpecDefaults(config map[string]any, userConfigSpec map[string]any) {
	connSpec, _ := userConfigSpec["connectionSpecification"].(map[string]any)
	if connSpec == nil {
		return
	}
	properties, _ := connSpec["properties"].(map[string]any)
	required, _ := connSpec["required"].([]any)

	// Required fields with explicit defaults.
	for _, r := range required {
		fieldName, ok := r.(string)
		if !ok {
			continue
		}
		if _, set := config[fieldName]; set {
			continue
		}
		propSpec, ok := properties[fieldName].(map[string]any)
		if !ok {
			continue
		}
		if def, ok := propSpec["default"]; ok && def != nil {
			config[fieldName] = def
		}
	}

	// Dynamic-date fields (regardless of required-ness).
	for fieldName, propAny := range properties {
		if _, set := config[fieldName]; set {
			continue
		}
		if !dynamicDateFieldPatterns[fieldName] {
			continue
		}
		propSpec, ok := propAny.(map[string]any)
		if !ok {
			continue
		}
		if propSpec["type"] != "string" || propSpec["format"] != "date-time" {
			continue
		}
		dt := time.Now().UTC().AddDate(0, 0, -730)
		pattern, _ := propSpec["pattern"].(string)
		switch {
		case pattern != "" && !patternHas(pattern, "{6}", "{3}", `\.`, ".[0-9]"):
			config[fieldName] = dt.Format("2006-01-02T15:04:05Z")
		case pattern != "" && patternContains(pattern, "{3}") && !patternContains(pattern, "{6}"):
			config[fieldName] = dt.Format("2006-01-02T15:04:05.000Z")
		default:
			// matches Python's strftime("%Y-%m-%dT%H:%M:%S.%fZ"): microseconds.
			config[fieldName] = dt.Format("2006-01-02T15:04:05.000000Z")
		}
	}
}

func patternHas(pattern string, anyOf ...string) bool {
	for _, s := range anyOf {
		if patternContains(pattern, s) {
			return true
		}
	}
	return false
}

func patternContains(pattern, substr string) bool {
	for i := 0; i+len(substr) <= len(pattern); i++ {
		if pattern[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
