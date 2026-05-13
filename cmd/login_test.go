package cmd

import (
	"strings"
	"testing"
)

func TestObfuscateSecret(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"short fully hidden", "abc", "***"},
		{"exactly 4 fully hidden", "abcd", "****"},
		{"5 chars shows last 4", "abcde", "*bcde"},
		{"long shows last 4", "abcdefghij", "******ghij"},
		{"typical secret", "sk-live-abcdef0123456789WXYZ", "************************WXYZ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := obfuscateSecret(c.in)
			if got != c.want {
				t.Errorf("obfuscateSecret(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestObfuscateSecret_AlwaysHidesMost(t *testing.T) {
	// Quick property check — for any non-trivial secret, no more than 4
	// non-asterisk characters should appear in the output.
	for _, in := range []string{"hunter2", "password", "thisIsASecret", strings.Repeat("x", 64)} {
		got := obfuscateSecret(in)
		visible := 0
		for _, r := range got {
			if r != '*' {
				visible++
			}
		}
		if visible > 4 {
			t.Errorf("obfuscateSecret(%q) leaked %d non-asterisk chars: %q", in, visible, got)
		}
	}
}
