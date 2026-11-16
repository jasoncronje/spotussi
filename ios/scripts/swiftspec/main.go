// Command swiftspec rewrites api/openapi.yaml into a form Swift OpenAPI
// Generator can use, for ios/scripts/gen-api.sh.
//
// The spec says "this or null" as
//
//	oneOf:
//	  - $ref: '#/components/schemas/ResolvedRange'
//	  - type: 'null'
//
// The generator skips the null branch's schema type and then drops the whole
// property, so the Swift type would lose the field and, with
// `additionalProperties: false`, refuse the response. Here each such
// property becomes its other branch and leaves its object's `required`
// list. The generated property is then an optional, which decodes both a
// missing key and an explicit null.
//
//	go run ./ios/scripts/swiftspec < api/openapi.yaml > spec.json
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"go.yaml.in/yaml/v3"
)

func main() {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err)
	}
	var doc any
	if err := yaml.Unmarshal(in, &doc); err != nil {
		fail(err)
	}
	doc = rewrite(doc)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "swiftspec:", err)
	os.Exit(1)
}

// rewrite walks the document, bottom-up, and fixes every object schema's
// nullable properties.
func rewrite(node any) any {
	switch n := node.(type) {
	case []any:
		for i, v := range n {
			n[i] = rewrite(v)
		}
		return n
	case map[string]any:
		for k, v := range n {
			n[k] = rewrite(v)
		}
		props, ok := n["properties"].(map[string]any)
		if !ok {
			return n
		}
		nullable := map[string]bool{}
		for name, schema := range props {
			if s, changed := withoutNull(schema); changed {
				props[name] = s
				nullable[name] = true
			}
		}
		if required, ok := n["required"].([]any); ok && len(nullable) > 0 {
			kept := make([]any, 0, len(required))
			for _, r := range required {
				if name, _ := r.(string); !nullable[name] {
					kept = append(kept, r)
				}
			}
			if len(kept) == 0 {
				delete(n, "required")
			} else {
				n["required"] = kept
			}
		}
		return n
	default:
		return node
	}
}

// withoutNull returns the schema with a `type: 'null'` branch removed from
// its oneOf or anyOf, and whether it had one. A single remaining branch
// replaces the oneOf.
func withoutNull(schema any) (any, bool) {
	s, ok := schema.(map[string]any)
	if !ok {
		return schema, false
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		branches, ok := s[key].([]any)
		if !ok {
			continue
		}
		rest := make([]any, 0, len(branches))
		for _, b := range branches {
			if m, ok := b.(map[string]any); ok && len(m) == 1 && m["type"] == "null" {
				continue
			}
			rest = append(rest, b)
		}
		if len(rest) == len(branches) {
			return schema, false
		}
		out := map[string]any{}
		for k, v := range s {
			if k != key {
				out[k] = v
			}
		}
		if len(rest) != 1 {
			out[key] = rest
			return out, true
		}
		only, ok := rest[0].(map[string]any)
		if !ok {
			return schema, false
		}
		if ref, isRef := only["$ref"]; isRef {
			// A bare $ref generates the referenced type itself. The
			// property's description is left behind: the spec has it.
			return map[string]any{"$ref": ref}, true
		}
		for k, v := range only {
			out[k] = v
		}
		return out, true
	}
	return schema, false
}
