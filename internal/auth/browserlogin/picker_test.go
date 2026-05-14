package browserlogin

import (
	"bytes"
	"strings"
	"testing"
)

func threeOrgs() []organizationListItem {
	return []organizationListItem{
		{ID: "11111111-1111-1111-1111-111111111111", OrganizationName: "Alpha"},
		{ID: "22222222-2222-2222-2222-222222222222", OrganizationName: "Beta"},
		{ID: "33333333-3333-3333-3333-333333333333", OrganizationName: "Gamma"},
	}
}

func TestPickOrganization_ValidChoice(t *testing.T) {
	orgs := threeOrgs()
	stdin := bytes.NewBufferString("2\n")
	stderr := &bytes.Buffer{}

	got, err := pickOrganization(stdin, stderr, orgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != orgs[1].ID {
		t.Fatalf("got %q, want %q (second org)", got, orgs[1].ID)
	}

	out := stderr.String()
	if !strings.Contains(out, "1) Alpha") {
		t.Errorf("stderr missing Alpha line: %q", out)
	}
	if !strings.Contains(out, "2) Beta") {
		t.Errorf("stderr missing Beta line: %q", out)
	}
	if !strings.Contains(out, "3) Gamma") {
		t.Errorf("stderr missing Gamma line: %q", out)
	}
	if !strings.Contains(out, "Enter choice [1-3]") {
		t.Errorf("stderr missing prompt: %q", out)
	}
}

func TestPickOrganization_OutOfRange(t *testing.T) {
	orgs := threeOrgs()
	stdin := bytes.NewBufferString("5\n")
	stderr := &bytes.Buffer{}

	_, err := pickOrganization(stdin, stderr, orgs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error %q does not mention 'out of range'", err.Error())
	}
}

func TestPickOrganization_NonNumeric(t *testing.T) {
	orgs := threeOrgs()
	stdin := bytes.NewBufferString("abc\n")
	stderr := &bytes.Buffer{}

	_, err := pickOrganization(stdin, stderr, orgs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not a number") {
		t.Errorf("error %q does not mention 'not a number'", err.Error())
	}
}

func TestPickOrganization_NilInputs(t *testing.T) {
	orgs := threeOrgs()
	if _, err := pickOrganization(nil, &bytes.Buffer{}, orgs); err == nil {
		t.Error("expected error for nil stdin")
	}
	if _, err := pickOrganization(&bytes.Buffer{}, nil, orgs); err == nil {
		t.Error("expected error for nil stderr")
	}
}

func TestPickOrganization_EmptyOrgs(t *testing.T) {
	if _, err := pickOrganization(&bytes.Buffer{}, &bytes.Buffer{}, nil); err == nil {
		t.Error("expected error for empty orgs slice")
	}
}
