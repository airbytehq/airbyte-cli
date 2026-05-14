package browser

import (
	"reflect"
	"testing"
)

func TestOpenInvokesOpenFunc(t *testing.T) {
	var got string
	old := OpenFunc
	OpenFunc = func(url string) error {
		got = url
		return nil
	}
	defer func() { OpenFunc = old }()

	if err := Open("https://example.com/test"); err != nil {
		t.Fatalf("Open returned err: %v", err)
	}
	if got != "https://example.com/test" {
		t.Errorf("OpenFunc got %q, want https://example.com/test", got)
	}
}

func TestOpenFuncDefaultsToOpenDefault(t *testing.T) {
	// Confirm the package-level default is openDefault. Compare via function
	// pointers; we do not actually invoke openDefault to avoid launching a
	// real browser during tests.
	wantPtr := reflect.ValueOf(openDefault).Pointer()
	gotPtr := reflect.ValueOf(OpenFunc).Pointer()
	if gotPtr != wantPtr {
		t.Errorf("OpenFunc default should point at openDefault (want %x, got %x)", wantPtr, gotPtr)
	}
}
