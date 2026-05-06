package resources

import "encoding/json"

// workspaceListPage is the streaming pagination wrapper used by
// listWorkspaces. Data stays as []json.RawMessage so all upstream fields are
// preserved verbatim when forwarding pages to the caller.
type workspaceListPage struct {
	Data []json.RawMessage `json:"data"`
	Next *string           `json:"next"`
}
