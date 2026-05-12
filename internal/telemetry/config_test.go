package telemetry

import "testing"

func TestResolveMode(t *testing.T) {
	cases := []struct {
		name            string
		envVar          string
		settingsEnabled bool
		want            Mode
	}{
		{"settings on, env unset", "", true, ModeBasic},
		{"settings off, env unset", "", false, ModeDisabled},
		{"env disabled overrides on", "disabled", true, ModeDisabled},
		{"env DISABLED case-insensitive", "DISABLED", true, ModeDisabled},
		{"env basic does not override off", "basic", false, ModeDisabled},
		{"env junk falls through to settings", "garbage", true, ModeBasic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AIRBYTE_TELEMETRY_MODE", tc.envVar)
			if got := ResolveMode(tc.settingsEnabled); got != tc.want {
				t.Errorf("ResolveMode(%v) with env=%q = %q, want %q", tc.settingsEnabled, tc.envVar, got, tc.want)
			}
		})
	}
}
