package refcli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/nodelogicau/intentions-bench/internal/impl"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/workspace"
	"github.com/nodelogicau/intentions-bench/internal/yamlutil"
)

// prefixes are the id prefixes by gold kind.
var prefixes = map[string]string{
	"desire": "des", "intention": "int", "terminus": "int", "availability": "avl", "commitment": "cmt",
}

// dirs are the object directories by gold kind.
var dirs = map[string]string{
	"desire": "desires", "intention": "intentions", "terminus": "intentions", "availability": "availability", "commitment": "commitments",
}

// Check refuses a scenario set the pinned reference CLI does not accept:
// the binary must be the version the manifest pins, every seed must
// validate with no error, every seed key must name a seed object, and every
// scenario's gold, applied to a copy of its seed, must validate with no
// error. tmp holds the gold workspaces.
func (c CLI) Check(set *scenario.Set, scenarios []*scenario.Scenario, tmp string) error {
	var ps scenario.Problems
	version, _, err := c.Version()
	if err != nil {
		return err
	}
	if version != set.Manifest.ReferenceCLI {
		return fmt.Errorf("reference CLI %s is version %s; the scenario set pins %s (manifest.yaml reference_cli)", c.Path, version, set.Manifest.ReferenceCLI)
	}
	seeds := map[string]bool{}
	for _, s := range scenarios {
		if seeds[s.Seed] {
			continue
		}
		seeds[s.Seed] = true
		dir := filepath.Join(set.Root, s.Seed)
		fs, err := c.Validate(dir)
		if err != nil {
			return err
		}
		for _, f := range Errors(fs) {
			ps = append(ps, fmt.Sprintf("seed %s: validate: %s %s: %s", s.Seed, f.Code, f.ID, f.Message))
		}
		ws, err := workspace.Load(dir)
		if err != nil {
			return err
		}
		for k, id := range s.Keys {
			if ws.Objects[id] == nil {
				ps = append(ps, fmt.Sprintf("seed %s: keys.yaml: %s names %s, which is not in the seed", s.Seed, k, id))
			}
		}
	}
	for _, s := range scenarios {
		dir := filepath.Join(tmp, s.ID)
		ids, err := BuildGold(set.Root, s, dir)
		if err != nil {
			ps = append(ps, fmt.Sprintf("scenario %s: gold: %v", s.ID, err))
			continue
		}
		if err := c.Index(dir); err != nil {
			return err
		}
		fs, err := c.Validate(dir)
		if err != nil {
			return err
		}
		names := map[string]string{}
		for k, id := range ids {
			names[id] = k
		}
		for _, f := range Errors(fs) {
			what := f.ID
			if k, ok := names[f.ID]; ok {
				what = "key " + k
			}
			msg := f.Message
			for id, k := range names {
				msg = strings.ReplaceAll(msg, id, "key "+k)
			}
			ps = append(ps, fmt.Sprintf("scenario %s: gold workspace fails validate: %s %s: %s", s.ID, f.Code, what, msg))
		}
	}
	if len(ps) > 0 {
		sort.Strings(ps)
		return ps
	}
	return nil
}

// BuildGold writes the gold workspace for a scenario into dir: a copy of the
// seed with every gold entry applied, new keys as new files and seed keys as
// edits to their files. It returns the id of every key. Placements are not
// built: `placed` is scored but leaves the gold workspace as the seed has it.
func BuildGold(root string, s *scenario.Scenario, dir string) (map[string]string, error) {
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := run.CopyDir(filepath.Join(root, s.Seed), dir); err != nil {
		return nil, err
	}
	_ = os.Remove(filepath.Join(dir, "keys.yaml"))
	cfg, err := readConfig(dir)
	if err != nil {
		return nil, err
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		return nil, err
	}
	ids := map[string]string{}
	for k, id := range s.Keys {
		ids[k] = id
	}
	for _, g := range s.Gold {
		if _, ok := ids[g.Key]; ok {
			continue
		}
		if g.Kind == "commitment" {
			return nil, fmt.Errorf("key %s: a new commitment cannot be built; gold commitments must be seed keys", g.Key)
		}
		ids[g.Key] = prefixes[g.Kind] + "_" + uuid.Must(uuid.NewV7()).String()
	}
	ts := s.Now.UTC().Format(time.RFC3339)
	src := map[string]any{"author": cfg.author, "harness": impl.Harness}
	isTerminus := func(key string) bool {
		if g, ok := s.GoldKey(key); ok {
			return g.Kind == "terminus"
		}
		if o := ws.Objects[ids[key]]; o != nil {
			return o.IsTerminus()
		}
		return false
	}
	for _, g := range s.Gold {
		id := ids[g.Key]
		obj := map[string]any{}
		path := filepath.Join(dir, dirs[g.Kind], id+".yaml")
		if seed, ok := ws.Objects[id]; ok {
			obj = seed.Fields
			path = filepath.Join(dir, seed.Path)
		} else {
			obj["id"] = id
			obj["subject"] = cfg.subject
			obj["title"] = g.Title
			if g.Title == "" {
				obj["title"] = g.Key
			}
			obj["serves"] = []any{}
			obj["source"] = src
			switch g.Kind {
			case "intention", "terminus":
				obj["stability"] = "tentative"
				obj["acknowledgements"] = []any{}
			case "availability":
				obj["scope"] = "personal"
				delete(obj, "serves")
			}
		}
		obj["timestamp"] = ts
		for name, v := range g.Fields {
			switch name {
			case "placed":
			case "retired":
				r := map[string]any{"kind": v, "timestamp": ts, "source": src}
				if g.AdoptedAs != "" {
					r["adopted_as"] = ids[g.AdoptedAs]
				}
				obj["retired"] = r
			case "serves":
				var refs []any
				for _, e := range v.([]any) {
					key, role := "", ""
					switch t := e.(type) {
					case string:
						key = t
					case map[string]any:
						key, _ = t["key"].(string)
						role, _ = t["role"].(string)
					}
					if role == "" {
						role = "in-order-to"
						if isTerminus(key) {
							role = "for-the-sake-of"
						}
					}
					refs = append(refs, map[string]any{"id": ids[key], "role": role})
				}
				obj["serves"] = refs
			case "window":
				w, _ := v.(map[string]any)
				out := map[string]any{}
				for k, e := range w {
					out[k] = e
				}
				if rel, ok := w["relative"].(map[string]any); ok {
					r := map[string]any{}
					for k, e := range rel {
						r[k] = e
					}
					r["target"] = ids[rel["target"].(string)]
					out["relative"] = r
				}
				obj["window"] = out
			default:
				obj[name] = v
			}
		}
		if err := writeYAML(path, obj); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

type config struct{ subject, author string }

func readConfig(dir string) (config, error) {
	b, err := os.ReadFile(filepath.Join(dir, "intentions.yaml"))
	if err != nil {
		return config{}, err
	}
	v, err := yamlutil.Unmarshal(b)
	if err != nil {
		return config{}, err
	}
	m, _ := v.(map[string]any)
	defaults, _ := m["defaults"].(map[string]any)
	source, _ := defaults["source"].(map[string]any)
	c := config{}
	c.subject, _ = defaults["subject"].(string)
	c.author, _ = source["author"].(string)
	if c.subject == "" || c.author == "" {
		return c, fmt.Errorf("seed intentions.yaml needs defaults.subject and defaults.source.author")
	}
	return c, nil
}

func writeYAML(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
