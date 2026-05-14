package browserlogin

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// pickOrganization renders a numbered list of orgs to stderr, reads a single
// integer from stdin, and returns the chosen org's ID.
//
// The caller is responsible for ensuring stdin is interactive — this function
// does not perform TTY detection (kept pure so it is trivially testable with
// bytes.Buffer in tests).
func pickOrganization(stdin io.Reader, stderr io.Writer, orgs []organizationListItem) (string, error) {
	if stdin == nil || stderr == nil {
		return "", errors.New("picker: stdin and stderr required")
	}
	if len(orgs) == 0 {
		return "", errors.New("picker: no organizations to choose from")
	}

	fmt.Fprintln(stderr, "You belong to multiple organizations. Choose one:")
	for i, o := range orgs {
		fmt.Fprintf(stderr, "  %d) %s  (%s)\n", i+1, o.OrganizationName, o.ID)
	}
	fmt.Fprintf(stderr, "Enter choice [1-%d]: ", len(orgs))

	reader := bufio.NewReader(stdin)
	raw, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("reading choice: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("picker: no input provided")
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return "", fmt.Errorf("picker: invalid choice %q (not a number)", raw)
	}
	if n < 1 || n > len(orgs) {
		return "", fmt.Errorf("picker: choice %d out of range [1, %d]", n, len(orgs))
	}
	return orgs[n-1].ID, nil
}
