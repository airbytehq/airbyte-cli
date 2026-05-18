// extract-schemas reads api/*.json (OpenAPI specs) and the operations
// declared by internal/resources, and emits internal/spec/extracted_gen.go
// containing only the request/response schemas referenced by the CLI. $refs
// are resolved into self-contained subtrees so runtime lookup is just a map
// hit.
//
// Run with: go generate ./... (driven by //go:generate in internal/spec).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/airbytehq/airbyte-cli/internal/registry"
	"github.com/airbytehq/airbyte-cli/internal/resources"
)

const (
	apiDir     = "api"
	outputFile = "internal/spec/extracted_gen.go"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "extract-schemas: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	// Find the project root by walking up from CWD until we see go.mod.
	root, err := findProjectRoot()
	if err != nil {
		return err
	}

	// Load all spec files in api/. They share the same shape, so we walk
	// each in turn looking up routes.
	specs, err := loadSpecs(filepath.Join(root, apiDir))
	if err != nil {
		return fmt.Errorf("loading specs: %w", err)
	}
	if len(specs) == 0 {
		return fmt.Errorf("no spec files found in %s", apiDir)
	}

	// Enumerate every SpecRef from the live registry.
	registry.Reset()
	resources.RegisterAll()
	refs := collectRefs()

	// Resolve each ref against the loaded specs.
	extracted := map[string]extractedRoute{}
	var missing []string
	for _, ref := range refs {
		route, ok := lookupRoute(specs, ref)
		if !ok {
			missing = append(missing, ref.Method+" "+ref.Path)
			continue
		}
		extracted[ref.Method+" "+ref.Path] = route
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("the following SpecRefs were not found in any api/*.json:\n  - %s", strings.Join(missing, "\n  - "))
	}

	// Emit the generated Go file.
	out, err := emit(extracted)
	if err != nil {
		return fmt.Errorf("emitting generated file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, outputFile), out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outputFile, err)
	}

	fmt.Printf("extract-schemas: wrote %d route(s) to %s\n", len(extracted), outputFile)
	return nil
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod from current directory")
		}
		dir = parent
	}
}

// openAPISpec is a partial parse of an OpenAPI document — enough to walk
// paths and resolve internal $refs.
type openAPISpec struct {
	raw  map[string]any // full document for $ref resolution
	path string         // file path, for error messages
}

func loadSpecs(dir string) ([]*openAPISpec, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var specs []*openAPISpec
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", full, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", full, err)
		}
		specs = append(specs, &openAPISpec{raw: doc, path: full})
	}
	return specs, nil
}

func collectRefs() []registry.SpecRef {
	seen := map[string]bool{}
	var refs []registry.SpecRef
	for _, res := range registry.All() {
		for _, op := range res.Operations() {
			if op.SpecRef.IsZero() || op.SpecRef.IsInternal() {
				continue
			}
			key := op.SpecRef.Key()
			if seen[key] {
				continue
			}
			seen[key] = true
			refs = append(refs, op.SpecRef)
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Key() < refs[j].Key() })
	return refs
}

type extractedRoute struct {
	Path        string
	Method      string
	Summary     string
	Description string
	Parameters  json.RawMessage
	RequestBody json.RawMessage
	Response    json.RawMessage
}

func lookupRoute(specs []*openAPISpec, ref registry.SpecRef) (extractedRoute, bool) {
	method := strings.ToLower(ref.Method)
	for _, spec := range specs {
		paths, _ := spec.raw["paths"].(map[string]any)
		entry, ok := paths[ref.Path].(map[string]any)
		if !ok {
			continue
		}
		op, ok := entry[method].(map[string]any)
		if !ok {
			continue
		}
		return buildRoute(spec, ref, op), true
	}
	return extractedRoute{}, false
}

func buildRoute(spec *openAPISpec, ref registry.SpecRef, op map[string]any) extractedRoute {
	r := extractedRoute{Path: ref.Path, Method: ref.Method}
	if v, ok := op["summary"].(string); ok {
		r.Summary = v
	}
	if v, ok := op["description"].(string); ok {
		r.Description = v
	}
	if params, ok := op["parameters"].([]any); ok && len(params) > 0 {
		resolved := resolveValue(spec, params, map[string]bool{})
		r.Parameters = mustMarshal(resolved)
	}
	if rb, ok := op["requestBody"].(map[string]any); ok {
		resolved := resolveValue(spec, rb, map[string]bool{})
		r.RequestBody = mustMarshal(extractContentSchema(resolved))
	}
	if responses, ok := op["responses"].(map[string]any); ok {
		// Prefer 200; fall back to first 2xx; else first key.
		key := pickResponseKey(responses)
		if key != "" {
			resp := resolveValue(spec, responses[key], map[string]bool{})
			r.Response = mustMarshal(extractContentSchema(resp))
		}
	}
	return r
}

func pickResponseKey(responses map[string]any) string {
	if _, ok := responses["200"]; ok {
		return "200"
	}
	keys := make([]string, 0, len(responses))
	for k := range responses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.HasPrefix(k, "2") {
			return k
		}
	}
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// extractContentSchema unwraps OpenAPI's "content"/"application/json"/"schema"
// nesting if present, returning just the JSON schema. Falls back to the input
// if the structure isn't recognized.
func extractContentSchema(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	content, ok := m["content"].(map[string]any)
	if !ok {
		return m
	}
	js, ok := content["application/json"].(map[string]any)
	if !ok {
		// Fall back to whichever content type is present.
		for _, val := range content {
			if cm, ok := val.(map[string]any); ok {
				js = cm
				break
			}
		}
	}
	if js == nil {
		return m
	}
	if schema, ok := js["schema"]; ok {
		return schema
	}
	return js
}

// resolveValue walks a JSON-decoded value tree, replacing every {"$ref": "..."}
// node with the referenced object (also recursively resolved). visited tracks
// the current resolution stack to break cycles — when a cycle is detected the
// $ref is left unresolved (as a marker the runtime can still inspect).
//
// Siblings to $ref (allowed in OpenAPI 3.1 / JSON Schema 2020-12) are merged
// onto the resolved target so per-call overrides like `description` are
// preserved.
func resolveValue(spec *openAPISpec, v any, visited map[string]bool) any {
	switch t := v.(type) {
	case map[string]any:
		if ref, isRef := t["$ref"].(string); isRef {
			if visited[ref] {
				// cycle — return the bare $ref so consumers know the cycle exists.
				return map[string]any{"$ref": ref}
			}
			target, ok := derefPointer(spec, ref)
			if !ok {
				return t
			}
			next := copyVisited(visited)
			next[ref] = true
			resolved := resolveValue(spec, target, next)

			// Fast path: ref-only node, return resolved as-is.
			if len(t) == 1 {
				return resolved
			}

			// Merge sibling keys onto the resolved object.
			if rm, ok := resolved.(map[string]any); ok {
				merged := make(map[string]any, len(rm)+len(t))
				for k, val := range rm {
					merged[k] = val
				}
				for k, val := range t {
					if k == "$ref" {
						continue
					}
					merged[k] = resolveValue(spec, val, visited)
				}
				return merged
			}
			return resolved
		}
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[k] = resolveValue(spec, child, visited)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = resolveValue(spec, child, visited)
		}
		return out
	default:
		return v
	}
}

// derefPointer follows a JSON pointer like "#/components/schemas/Foo" inside
// the spec and returns the referenced node. Only same-document refs are
// supported (the only kind used by the Airbyte specs).
func derefPointer(spec *openAPISpec, ref string) (any, bool) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	var cur any = spec.raw
	for _, p := range parts {
		// JSON pointer escape: ~1 → "/", ~0 → "~"
		p = strings.ReplaceAll(p, "~1", "/")
		p = strings.ReplaceAll(p, "~0", "~")
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func copyVisited(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshaling extracted schema: %v", err))
	}
	return b
}

func emit(routes map[string]extractedRoute) ([]byte, error) {
	keys := make([]string, 0, len(routes))
	for k := range routes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteString("// Code generated by cmd/extract-schemas. DO NOT EDIT.\n\n")
	buf.WriteString("package spec\n\n")
	buf.WriteString("import \"encoding/json\"\n\n")
	buf.WriteString("// schemas is populated by the generator from api/*.json. Keys are\n")
	buf.WriteString("// \"<METHOD> <path>\" (e.g. \"GET /api/v1/workspaces\"). This file is\n")
	buf.WriteString("// overwritten on every `go generate ./...` run.\n")
	buf.WriteString("var schemas = map[string]RouteSchema{\n")
	for _, k := range keys {
		r := routes[k]
		buf.WriteString(fmt.Sprintf("\t%q: {\n", k))
		buf.WriteString(fmt.Sprintf("\t\tPath:   %q,\n", r.Path))
		buf.WriteString(fmt.Sprintf("\t\tMethod: %q,\n", r.Method))
		if r.Summary != "" {
			buf.WriteString(fmt.Sprintf("\t\tSummary: %q,\n", r.Summary))
		}
		if r.Description != "" {
			buf.WriteString(fmt.Sprintf("\t\tDescription: %q,\n", r.Description))
		}
		if len(r.Parameters) > 0 {
			buf.WriteString(fmt.Sprintf("\t\tParameters: json.RawMessage(%s),\n", quoteForBacktick(r.Parameters)))
		}
		if len(r.RequestBody) > 0 {
			buf.WriteString(fmt.Sprintf("\t\tRequestBody: json.RawMessage(%s),\n", quoteForBacktick(r.RequestBody)))
		}
		if len(r.Response) > 0 {
			buf.WriteString(fmt.Sprintf("\t\tResponse: json.RawMessage(%s),\n", quoteForBacktick(r.Response)))
		}
		buf.WriteString("\t},\n")
	}
	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Return unformatted source on error so the file is still inspectable.
		return buf.Bytes(), fmt.Errorf("gofmt failed (raw output written): %w", err)
	}
	return formatted, nil
}

// quoteForBacktick renders a JSON byte slice as a Go raw-string literal. If
// the JSON contains a backtick, falls back to a regular double-quoted Go
// string so the literal stays valid.
func quoteForBacktick(b []byte) string {
	s := string(b)
	if !strings.Contains(s, "`") {
		return "`" + s + "`"
	}
	q, _ := json.Marshal(s)
	return string(q)
}
