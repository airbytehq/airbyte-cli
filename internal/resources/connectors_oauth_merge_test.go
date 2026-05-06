package resources

import (
	"reflect"
	"testing"
)

func TestSetNestedValue_Flat(t *testing.T) {
	obj := map[string]any{}
	setNestedValue(obj, []string{"a"}, 1)
	if obj["a"] != 1 {
		t.Errorf("expected a=1, got %v", obj["a"])
	}
}

func TestSetNestedValue_Nested(t *testing.T) {
	obj := map[string]any{}
	setNestedValue(obj, []string{"credentials", "auth_method", "client_id"}, "abc")
	want := map[string]any{
		"credentials": map[string]any{
			"auth_method": map[string]any{"client_id": "abc"},
		},
	}
	if !reflect.DeepEqual(obj, want) {
		t.Errorf("got %v, want %v", obj, want)
	}
}

func TestSetNestedValue_PreservesSiblings(t *testing.T) {
	obj := map[string]any{
		"credentials": map[string]any{"keep": true},
	}
	setNestedValue(obj, []string{"credentials", "client_id"}, "abc")
	creds := obj["credentials"].(map[string]any)
	if creds["keep"] != true {
		t.Errorf("expected sibling 'keep' preserved, got %v", creds)
	}
	if creds["client_id"] != "abc" {
		t.Errorf("expected client_id=abc, got %v", creds)
	}
}

func TestMergeOAuthCredentials_FlatFallback(t *testing.T) {
	// No advanced_auth → flat-merge auth payload onto config.
	got := mergeOAuthCredentials(
		map[string]any{}, // empty user_config_spec
		map[string]any{"existing": "kept"},
		map[string]any{"api_key": "secret"},
	)
	want := map[string]any{
		"existing": "kept",
		"api_key":  "secret",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeOAuthCredentials_AdvancedAuthNestedPaths(t *testing.T) {
	spec := map[string]any{
		"advanced_auth": map[string]any{
			"predicate_key":   []any{"credentials", "auth_method"},
			"predicate_value": "oauth2.0",
			"oauth_config_specification": map[string]any{
				"complete_oauth_output_specification": map[string]any{
					"properties": map[string]any{
						"access_token": map[string]any{
							"path_in_connector_config": []any{"credentials", "access_token"},
						},
						"refresh_token": map[string]any{
							"path_in_connector_config": []any{"credentials", "refresh_token"},
						},
					},
				},
				"complete_oauth_server_output_specification": map[string]any{
					"properties": map[string]any{
						"client_id": map[string]any{
							"path_in_connector_config": []any{"credentials", "client_id"},
						},
					},
				},
			},
		},
	}
	authPayload := map[string]any{
		"access_token":  "at-1",
		"refresh_token": "rt-1",
		"client_id":     "cid-1",
		"unrelated":     "should-be-ignored",
	}

	got := mergeOAuthCredentials(spec, map[string]any{}, authPayload)
	credentials, ok := got["credentials"].(map[string]any)
	if !ok {
		t.Fatalf("expected credentials map, got %v", got)
	}
	if credentials["auth_method"] != "oauth2.0" {
		t.Errorf("expected auth_method=oauth2.0, got %v", credentials["auth_method"])
	}
	if credentials["access_token"] != "at-1" {
		t.Errorf("expected access_token=at-1, got %v", credentials["access_token"])
	}
	if credentials["refresh_token"] != "rt-1" {
		t.Errorf("expected refresh_token=rt-1, got %v", credentials["refresh_token"])
	}
	if credentials["client_id"] != "cid-1" {
		t.Errorf("expected client_id=cid-1, got %v", credentials["client_id"])
	}
	// Unmapped auth payload keys should NOT leak into the config when
	// advanced_auth is in effect (we only place declared properties).
	if _, leaked := got["unrelated"]; leaked {
		t.Errorf("unmapped auth payload key leaked into config: %v", got)
	}
}

func TestMergeOAuthCredentials_DoesNotMutateInput(t *testing.T) {
	partial := map[string]any{"a": 1}
	mergeOAuthCredentials(map[string]any{}, partial, map[string]any{"b": 2})
	if _, leaked := partial["b"]; leaked {
		t.Errorf("input partial config was mutated: %v", partial)
	}
}

func TestApplySpecDefaults_RequiredFieldDefault(t *testing.T) {
	spec := map[string]any{
		"connectionSpecification": map[string]any{
			"required": []any{"region"},
			"properties": map[string]any{
				"region": map[string]any{"default": "us-east-1"},
			},
		},
	}
	cfg := map[string]any{}
	applySpecDefaults(cfg, spec)
	if cfg["region"] != "us-east-1" {
		t.Errorf("expected region=us-east-1, got %v", cfg["region"])
	}
}

func TestApplySpecDefaults_DoesNotOverrideExisting(t *testing.T) {
	spec := map[string]any{
		"connectionSpecification": map[string]any{
			"required": []any{"region"},
			"properties": map[string]any{
				"region": map[string]any{"default": "us-east-1"},
			},
		},
	}
	cfg := map[string]any{"region": "eu-west-1"}
	applySpecDefaults(cfg, spec)
	if cfg["region"] != "eu-west-1" {
		t.Errorf("existing value overwritten: got %v", cfg["region"])
	}
}

func TestApplySpecDefaults_DynamicStartDate(t *testing.T) {
	spec := map[string]any{
		"connectionSpecification": map[string]any{
			"properties": map[string]any{
				"start_date": map[string]any{
					"type":   "string",
					"format": "date-time",
				},
			},
		},
	}
	cfg := map[string]any{}
	applySpecDefaults(cfg, spec)
	val, ok := cfg["start_date"].(string)
	if !ok || val == "" {
		t.Fatalf("expected start_date to be filled in, got %v", cfg["start_date"])
	}
	// Should have microsecond precision by default (no pattern).
	if len(val) < 20 {
		t.Errorf("start_date %q seems malformed", val)
	}
}
