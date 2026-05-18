package browserlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/airbytehq/airbyte-cli/internal/client"
)

// BootstrapResult is the trio of values written to ~/.airbyte-cli/settings.json
// after a successful browser login.
type BootstrapResult struct {
	ClientID       string
	ClientSecret   string
	OrganizationID string
}

// BootstrapOptions configures the Bootstrap call.
type BootstrapOptions struct {
	APIHost       string        // e.g. https://api.airbyte.ai (no trailing /api/v1)
	AccessToken   string        // Keycloak access token from RunOAuthFlow
	OrgIDOverride string        // value of --org-id flag, or "" for autodetect
	Stdin         io.Reader     // for the multi-org picker
	Stderr        io.Writer     // for the multi-org picker UI
	StdinIsTTY    bool          // caller signals whether interactive prompts are OK
	CLIVersion    string        // value of cmd.Version (defaults to "dev")
	HTTPClient    *http.Client  // test seam; nil → default
	Timeout       time.Duration // per-request timeout (default 30s if zero and HTTPClient nil)
}

const (
	APIBasePath           = "/api/v1"
	enrollmentStatusPath  = "/internal/account/enrollment-status"
	organizationsListPath = "/internal/account/organizations"
	applicationsPath      = "/internal/account/applications"
	headerOrgID           = "X-Organization-Id"
	headerAgentCLI        = "X-ADP-Agent-CLI"
)

// enrollmentStatusResponse mirrors sonar's EnrollmentStatusResponse, only the
// fields the CLI consumes. Extra fields are ignored by json.Unmarshal.
type enrollmentStatusResponse struct {
	IsNewUser        bool   `json:"is_new_user"`
	IsEnrolled       bool   `json:"is_enrolled"`
	IsInstanceAdmin  bool   `json:"is_instance_admin"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name,omitempty"`
}

// organizationListItem mirrors sonar's Organization (within OrganizationsListResponse).
type organizationListItem struct {
	ID               string `json:"id"`
	OrganizationName string `json:"organization_name"`
	FirstWorkspaceID string `json:"first_workspace_id,omitempty"`
}

// organizationsListResponse mirrors sonar's OrganizationsListResponse.
type organizationsListResponse struct {
	Organizations   []organizationListItem `json:"organizations"`
	IsInstanceAdmin bool                   `json:"is_instance_admin"`
}

// applicationResponse mirrors sonar's Application (subset).
type applicationResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Bootstrap calls the sonar bootstrap endpoints and returns the
// (client_id, client_secret, organization_id) trio.
//
// Sequence:
//  1. GET enrollment-status (no org header) → captures initialOrgID.
//  2. If OrgIDOverride is empty: GET organizations (with X-Organization-Id:
//     initialOrgID). 1 org → auto-select. >1 org → interactive picker (if
//     stdin is a TTY) or actionable error (if not).
//  3. POST applications (with X-Organization-Id: chosenOrgID) → returns
//     client_id + client_secret.
//
// All sonar HTTP error shapes are mapped to *client.APIError so the CLI's
// existing exit-code logic works without modification.
func Bootstrap(ctx context.Context, opts *BootstrapOptions) (*BootstrapResult, error) {
	if opts == nil {
		return nil, errors.New("bootstrap: nil options")
	}
	if opts.APIHost == "" {
		return nil, errors.New("bootstrap: APIHost required")
	}
	if opts.AccessToken == "" {
		return nil, errors.New("bootstrap: AccessToken required")
	}

	// Step 1: enrollment-status. The route auto-selects the first org when no
	// X-Organization-Id header is set, so we omit it here.
	var enr enrollmentStatusResponse
	if err := opts.do(ctx, http.MethodGet, enrollmentStatusPath, "", nil, &enr); err != nil {
		return nil, err
	}
	if !enr.IsEnrolled {
		return nil, client.NewValidationError(
			"complete onboarding at https://app.airbyte.ai before running login",
			"open the URL in a browser, finish the enrollment flow, then re-run airbyte agents login",
		)
	}

	initialOrgID := enr.OrganizationID

	// Steps 2 & 3: pick organization (unless overridden via --org-id).
	chosenOrgID := opts.OrgIDOverride
	if chosenOrgID == "" {
		var orgs organizationsListResponse
		if err := opts.do(ctx, http.MethodGet, organizationsListPath, initialOrgID, nil, &orgs); err != nil {
			return nil, err
		}
		switch {
		case len(orgs.Organizations) == 0 && orgs.IsInstanceAdmin:
			return nil, &client.APIError{
				Type:       "validation_error",
				Message:    "instance admin login not supported from CLI; use a regular user account",
				StatusCode: 400,
			}
		case len(orgs.Organizations) == 0:
			return nil, &client.APIError{
				Type:       "validation_error",
				Message:    "no organizations found for this account",
				StatusCode: 400,
				Hint:       "complete onboarding at https://app.airbyte.ai before running login",
			}
		case len(orgs.Organizations) == 1:
			chosenOrgID = orgs.Organizations[0].ID
		default:
			if !opts.StdinIsTTY {
				return nil, &client.APIError{
					Type:       "validation_error",
					Message:    "you belong to multiple organizations and stdin is not a TTY",
					StatusCode: 400,
					Hint:       buildOrgListHint(orgs.Organizations),
				}
			}
			picked, err := pickOrganization(opts.Stdin, opts.Stderr, orgs.Organizations)
			if err != nil {
				return nil, err
			}
			chosenOrgID = picked
		}
	}

	// Step 4: POST applications.
	var app applicationResponse
	if err := opts.do(ctx, http.MethodPost, applicationsPath, chosenOrgID, nil, &app); err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) {
			// 403 → likely Stigg-subscription gate fired.
			if apiErr.StatusCode == http.StatusForbidden {
				return nil, client.NewValidationError(
					"your organization is not enrolled or its subscription is inactive",
					"complete onboarding at https://app.airbyte.ai, then re-run airbyte agents login",
				)
			}
			// 400 "specify target organization" — we already resolved an
			// orgID and put it in the X-Organization-Id header, so seeing
			// this means the server disagreed. Wrap so the user understands
			// they aren't missing a flag.
			if apiErr.StatusCode == http.StatusBadRequest &&
				strings.Contains(strings.ToLower(apiErr.Message), "specify target organization") {
				return nil, fmt.Errorf("server demanded an organization header even though one was supplied (org-id=%s): %w", chosenOrgID, err)
			}
		}
		return nil, err
	}

	return &BootstrapResult{
		ClientID:       app.ClientID,
		ClientSecret:   app.ClientSecret,
		OrganizationID: chosenOrgID,
	}, nil
}

// buildOrgListHint renders the available orgs for inclusion in an APIError
// Hint when stdin isn't a TTY and the user must rerun with --org-id.
func buildOrgListHint(orgs []organizationListItem) string {
	var sb strings.Builder
	sb.WriteString("re-run with --org-id <uuid>. Available organizations:")
	for _, o := range orgs {
		fmt.Fprintf(&sb, "\n  - %s (%s)", o.OrganizationName, o.ID)
	}
	return sb.String()
}

// do executes an authed request to opts.APIHost + APIBasePath + path. If `out`
// is non-nil, the response body is decoded into it. Non-2xx responses are
// returned as *client.APIError (matching the CLI's exit-code mapping).
func (o *BootstrapOptions) do(ctx context.Context, method, path, orgID string, body []byte, out interface{}) error {
	url := strings.TrimRight(o.APIHost, "/") + APIBasePath + path
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("building %s request: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+o.AccessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if orgID != "" {
		req.Header.Set(headerOrgID, orgID)
	}
	version := o.CLIVersion
	if version == "" {
		version = "dev"
	}
	req.Header.Set(headerAgentCLI, version)
	req.Header.Set("User-Agent", "airbyte-cli/"+version)

	httpClient := o.HTTPClient
	if httpClient == nil {
		timeout := o.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s response: %w", path, err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || len(raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decoding %s response: %w", path, err)
		}
		return nil
	}

	// Non-2xx → APIError. Lift "detail" or "message" out of the body if JSON.
	apiErr := &client.APIError{
		StatusCode: resp.StatusCode,
		Type:       statusToType(resp.StatusCode),
		Message:    fmt.Sprintf("%s %s returned %d", method, path, resp.StatusCode),
	}
	if len(raw) > 0 && json.Valid(raw) {
		apiErr.Detail = json.RawMessage(raw)
		var lifted struct {
			Detail  string `json:"detail"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &lifted) == nil {
			if lifted.Detail != "" {
				apiErr.Message = lifted.Detail
			} else if lifted.Message != "" {
				apiErr.Message = lifted.Message
			}
		}
	}
	return apiErr
}

func statusToType(code int) string {
	switch {
	case code == 401:
		return "unauthorized"
	case code == 403:
		return "forbidden"
	case code == 400 || code == 422:
		return "validation_error"
	case code == 404:
		return "not_found"
	case code >= 500:
		return "server_error"
	default:
		return "api_error"
	}
}
