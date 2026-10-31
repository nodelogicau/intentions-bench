// Package workspace reads an Intentions Format workspace as files, per the
// format's layout, without any implementation's help: the harness scores
// what was written, not what an implementation reports.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/nodelogicau/intentions-bench/internal/yamlutil"
)

// Dirs maps each object directory to its type.
var Dirs = map[string]string{
	"desires":      "desire",
	"intentions":   "intention",
	"availability": "availability",
	"commitments":  "commitment",
	"resolutions":  "resolution",
}

// Object is one object file.
type Object struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Path string `json:"path"`
	// Fields is the file as decoded YAML.
	Fields map[string]any `json:"fields"`
}

// Workspace is every object in a workspace, by id.
type Workspace struct {
	Root    string
	Objects map[string]*Object
}

// Load reads every object file under root.
func Load(root string) (*Workspace, error) {
	ws := &Workspace{Root: root, Objects: map[string]*Object{}}
	for dir, typ := range Dirs {
		paths, err := filepath.Glob(filepath.Join(root, dir, "*.yaml"))
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			v, err := yamlutil.Unmarshal(b)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			m, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s: not a mapping", p)
			}
			id, _ := m["id"].(string)
			if id == "" {
				id = strings.TrimSuffix(filepath.Base(p), ".yaml")
			}
			rel, _ := filepath.Rel(root, p)
			ws.Objects[id] = &Object{ID: id, Type: typ, Path: rel, Fields: m}
		}
	}
	return ws, nil
}

// IDs returns the object ids, sorted.
func (ws *Workspace) IDs() []string {
	out := make([]string, 0, len(ws.Objects))
	for id := range ws.Objects {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Diff returns the ids of objects in after that are not in before, and of
// those in both whose files differ, each sorted.
func Diff(before, after *Workspace) (created, changed []string) {
	for _, id := range after.IDs() {
		a := after.Objects[id]
		b, ok := before.Objects[id]
		switch {
		case !ok:
			created = append(created, id)
		case !reflect.DeepEqual(a.Fields, b.Fields):
			changed = append(changed, id)
		}
	}
	return created, changed
}

// Str returns a string field, or "".
func (o *Object) Str(name string) string {
	s, _ := o.Fields[name].(string)
	return s
}

// Has reports whether a field is present and non-empty.
func (o *Object) Has(name string) bool {
	v, ok := o.Fields[name]
	if !ok || v == nil {
		return false
	}
	if l, ok := v.([]any); ok {
		return len(l) > 0
	}
	if m, ok := v.(map[string]any); ok {
		return len(m) > 0
	}
	return true
}

// IsTerminus reports whether an intention is a terminus: no serves, no
// window, no duration.
func (o *Object) IsTerminus() bool {
	return o.Type == "intention" && !o.Has("serves") && !o.Has("window") && !o.Has("duration")
}

// Rung is the rung an object stands on: desire, intention, terminus,
// availability, or commitment. A resolution record stands on none ("").
func (o *Object) Rung() string {
	switch {
	case o.Type == "resolution":
		return ""
	case o.IsTerminus():
		return "terminus"
	default:
		return o.Type
	}
}

// RetiredKind returns retired.kind, or "".
func (o *Object) RetiredKind() string {
	r, _ := o.Fields["retired"].(map[string]any)
	k, _ := r["kind"].(string)
	return k
}

// Timestamp returns the object's timestamp.
func (o *Object) Timestamp() (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, o.Str("timestamp"))
	return t, err == nil
}

// ServesIDs returns the ids an object's serves lists.
func (o *Object) ServesIDs() []string {
	list, _ := o.Fields["serves"].([]any)
	var out []string
	for _, e := range list {
		if m, ok := e.(map[string]any); ok {
			if id, ok := m["id"].(string); ok {
				out = append(out, id)
			}
		}
	}
	return out
}

// Reaches reports whether from reaches target through serves references,
// following superseded_by at a superseded link, as the grounding walk does.
func (ws *Workspace) Reaches(from, target string) bool {
	seen := map[string]bool{}
	queue := []string{from}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		o := ws.Objects[id]
		if o == nil {
			continue
		}
		for _, next := range o.ServesIDs() {
			if next == target {
				return true
			}
			queue = append(queue, next)
		}
		if r, ok := o.Fields["retired"].(map[string]any); ok {
			if s, ok := r["superseded_by"].(string); ok && id != from {
				if s == target {
					return true
				}
				queue = append(queue, s)
			}
		}
	}
	return false
}
