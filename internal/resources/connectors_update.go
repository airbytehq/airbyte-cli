package resources

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/airbytehq/airbyte-agent-cli/internal/client"
	"github.com/airbytehq/airbyte-agent-cli/internal/registry"
)

// connectorsUpdateOperation registers `connectors update`, a browser-launch
// command that resolves the target connector and opens the user's webapp
// credentials page. The CLI never accepts credentials directly — credential
// entry happens in the browser-based widget mounted on the credentials page
// (sonar/frontend/src/routes/organizations/$organizationId/credentials.tsx:1423).
//
// The SpecRef points at the conceptual PUT /api/v1/integrations/connectors/{id}
// route the webapp invokes after the user submits the edit form. The CLI does
// NOT call that endpoint — the value is informational and feeds
// `airbyte-agent schema connectors update`.
func connectorsUpdateOperation() registry.Operation {
	return registry.Operation{
		Name:        "update",
		Description: "Open the browser to edit a connector's credentials/config",
		Schema: registry.OperationSchema{
			Description: "Open the credentials page in your browser so you can edit an existing connector. The CLI never accepts credentials directly — entry happens in the browser-based widget.",
			Params: map[string]registry.ParamSchema{
				"name":      {Type: "string", Required: false, Description: "Connector name (requires workspace)"},
				"workspace": {Type: "string", Required: false, Description: "Workspace name (defaults to 'default' when used with name)"},
				"id":        {Type: "string", Required: false, Description: "Connector ID (alternative to name)"},
			},
		},
		SpecRef: registry.SpecRef{Path: "/api/v1/integrations/connectors/{id}", Method: "PUT"},
		Hooks: registry.OperationHooks{
			PreRun: resolveConnectorID,
		},
		Run: connectorsUpdate,
	}
}

// connectorsUpdate is the Run function for `connectors update`. After
// `resolveConnectorID` populates `params["id"]`, it builds the credentials-
// page URL, displays the human-readable message + a yes/no prompt, and only
// opens the browser on an explicit "yes". The prompt has a short timeout so
// non-interactive callers (MCP, CI, piped invocations) don't hang waiting
// for input that will never arrive — they simply receive the URL in the
// result map and can act on it themselves. The result always includes
// `browser_opened` so callers can tell which path ran.
func connectorsUpdate(ctx context.Context, c *client.Client, params map[string]any) (any, error) {
	id, _ := params["id"].(string)

	orgID := c.OrganizationID()
	if orgID == "" {
		return nil, client.NewValidationError(
			"no organization_id configured",
			"run 'airbyte-agent login' or set AIRBYTE_ORGANIZATION_ID",
		)
	}

	pageURL, err := credentialsPageURL(webAppBaseURL(), orgID)
	if err != nil {
		return nil, fmt.Errorf("building credentials page URL: %w", err)
	}

	name, _ := params["name"].(string)
	workspace, _ := params["workspace"].(string)
	message := updateMessageFor(name, workspace, id)

	opened := false
	if confirmOpenBrowser(message, pageURL) {
		openBrowser(pageURL)
		opened = true
	}

	return map[string]any{
		"url":            pageURL,
		"connector_id":   id,
		"message":        message,
		"browser_opened": opened,
	}, nil
}

// confirmOpenBrowserTimeout caps how long the confirmation prompt waits for
// stdin before defaulting to "no". Long enough for a TTY user glancing at
// the prompt to type a response; short enough that non-interactive callers
// don't hang noticeably. It is a var (not a const) so tests can shrink it.
var confirmOpenBrowserTimeout = 10 * time.Second

// confirmOpenBrowser prints the action message + a yes/no prompt to
// confirmWriter (stderr by default), then reads a single line from
// confirmReader (stdin by default) with a confirmOpenBrowserTimeout-bounded
// wait. Returns true ONLY on an exact "yes" (case-insensitive, whitespace-
// trimmed). Any other input, EOF, or a timeout returns false — the URL is
// still surfaced in the caller's result map so the user/agent can act on it
// independently. Declared as a var so tests can stub the whole prompt
// rather than driving the real stdin read.
var confirmOpenBrowser = func(message, url string) bool {
	fmt.Fprintln(confirmWriter, message)
	fmt.Fprintf(confirmWriter, "Open %s in your browser? Type 'yes' to confirm (skips after %s): ", url, confirmOpenBrowserTimeout)

	type readResult struct {
		line string
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := bufio.NewReader(confirmReader).ReadString('\n')
		ch <- readResult{line: line, err: err}
	}()

	select {
	case r := <-ch:
		confirmed := strings.EqualFold(strings.TrimSpace(r.line), "yes")
		switch {
		case confirmed:
			return true
		case r.err == io.EOF && strings.TrimSpace(r.line) == "":
			// Non-TTY callers (MCP, piped subprocess) typically hit EOF
			// instantly. Skip the chatty "(not opening browser)" notice
			// since the result map already conveys the outcome.
			return false
		default:
			fmt.Fprintln(confirmWriter, "(not opening browser)")
			return false
		}
	case <-time.After(confirmOpenBrowserTimeout):
		fmt.Fprintln(confirmWriter, "\n(timed out; not opening browser)")
		return false
	}
}

// updateMessageFor crafts the user-facing instruction printed alongside the
// browser launch. The lead sentence makes it explicit that the CLI does not
// itself edit the connector — the link (returned in the `url` field) is the
// only path, and the trailing clause points the user at the pencil icon
// inside the credentials page. The most specific phrasing
// (name + workspace + id) is preferred; the id-only fallback covers the
// --id invocation path.
const updateDisclaimer = "Connectors cannot be edited through the CLI. Visit the link below to update the connector config"

func updateMessageFor(name, workspace, id string) string {
	switch {
	case name != "" && workspace != "":
		return fmt.Sprintf("%s — find connector %q (id %s) in workspace %q on the credentials page and click the pencil icon to edit.", updateDisclaimer, name, id, workspace)
	case name != "":
		return fmt.Sprintf("%s — find connector %q (id %s) on the credentials page and click the pencil icon to edit.", updateDisclaimer, name, id)
	default:
		return fmt.Sprintf("%s — find the connector with id %s on the credentials page and click the pencil icon to edit.", updateDisclaimer, id)
	}
}

// credentialsPageURL builds the webapp URL the browser opens for credential
// edits: <baseURL>/organizations/<orgID>/credentials. The base URL may carry
// an existing path; a trailing slash is collapsed so the result never
// contains "//organizations/...". orgID is URL-path-escaped to defend against
// path injection if the org id ever contains slashes or other reserved
// characters. Both u.Path and u.RawPath are set so url.URL.String preserves
// the already-escaped form instead of percent-encoding the escape sequences
// a second time.
func credentialsPageURL(baseURL, orgID string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parsing web app URL: %w", err)
	}
	trimmed := strings.TrimRight(u.Path, "/")
	escapedOrg := url.PathEscape(orgID)
	u.Path = trimmed + "/organizations/" + orgID + "/credentials"
	u.RawPath = trimmed + "/organizations/" + escapedOrg + "/credentials"
	return u.String(), nil
}
