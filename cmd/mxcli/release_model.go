// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/modelsdk/mpr"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// loadReleaseModel reads the persistent part of a model from its stored form:
// every entity, attribute and association with its GUID (ako/mxcli#2424).
// The GUID is read from the stored bytes because it is what the runtime keys
// the database on, and mxcli's own model types do not carry it.
func loadReleaseModel(mprPath string) (*projectreport.ReleaseModel, error) {
	r, err := mpr.Open(mprPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	type module struct {
		name        string
		marketplace bool
	}
	modules := map[string]module{}
	mods, err := r.ListRawUnitsByType("Projects$ModuleImpl")
	if err != nil {
		return nil, err
	}
	for _, u := range mods {
		var d bson.D
		if err := bson.Unmarshal(u.Contents, &d); err != nil {
			return nil, fmt.Errorf("module unit %s: %w", u.ID, err)
		}
		fromStore, _ := field(d, "FromAppStore").(bool)
		modules[string(u.ID)] = module{name: str(field(d, "Name")), marketplace: fromStore}
	}

	dms, err := r.ListRawUnitsByType("DomainModels$DomainModel")
	if err != nil {
		return nil, err
	}
	m := &projectreport.ReleaseModel{}
	type rawEntity struct {
		idx         int
		persistable *bool  // set by NoGeneralization
		generalizes string // Generalization target, qualified
		external    bool
	}
	var raws []rawEntity
	byID := map[string]int{}   // entity $ID -> index in m.Entities
	byName := map[string]int{} // qualified name -> index
	type rawAssoc struct {
		a        projectreport.RelAssoc
		from, to string // entity $IDs
		toName   string // cross-association: the target by name
	}
	var assocs []rawAssoc

	for _, u := range dms {
		mod, ok := modules[string(u.ContainerID)]
		if !ok {
			continue
		}
		var dm bson.D
		if err := bson.Unmarshal(u.Contents, &dm); err != nil {
			return nil, fmt.Errorf("domain model of %s: %w", mod.name, err)
		}
		for _, ev := range elements(field(dm, "Entities")) {
			e := projectreport.RelEntity{
				GUID: binHex(field(ev, "GUID")), ID: binHex(field(ev, "$ID")),
				Name: mod.name + "." + str(field(ev, "Name")), Module: mod.name, Marketplace: mod.marketplace,
			}
			re := rawEntity{external: field(ev, "Source") != nil}
			if g, ok := field(ev, "MaybeGeneralization").(bson.D); ok {
				switch str(field(g, "$Type")) {
				case "DomainModels$Generalization":
					re.generalizes = str(field(g, "Generalization"))
				default:
					p, ok := field(g, "Persistable").(bool)
					if !ok {
						p = true // absent in older formats: persistable was the default
					}
					re.persistable = &p
				}
			}
			e.Generalization = re.generalizes
			for _, av := range elements(field(ev, "Attributes")) {
				a := projectreport.RelAttr{GUID: binHex(field(av, "GUID")), Name: str(field(av, "Name"))}
				if t, ok := field(av, "NewType").(bson.D); ok {
					a.Type = strings.TrimSuffix(strings.TrimPrefix(str(field(t, "$Type")), "DomainModels$"), "AttributeType")
					if n, ok := bsonInt(field(t, "Length")); ok {
						a.Length = n
					}
				}
				if v, ok := field(av, "Value").(bson.D); ok {
					a.Calculated = str(field(v, "$Type")) == "DomainModels$CalculatedValue"
				}
				e.Attributes = append(e.Attributes, a)
			}
			re.idx = len(m.Entities)
			byID[e.ID] = re.idx
			byName[e.Name] = re.idx
			m.Entities = append(m.Entities, e)
			raws = append(raws, re)
		}
		for _, av := range elements(field(dm, "Associations")) {
			assocs = append(assocs, rawAssoc{
				a:    assocOf(av, mod.name),
				from: binHex(field(av, "ParentPointer")), to: binHex(field(av, "ChildPointer")),
			})
		}
		for _, av := range elements(field(dm, "CrossAssociations")) {
			assocs = append(assocs, rawAssoc{
				a:    assocOf(av, mod.name),
				from: binHex(field(av, "ParentPointer")), toName: str(field(av, "Child")),
			})
		}
	}

	// Persistence follows the generalization chain to its root, which may be
	// a System entity.
	var persistent func(i int, seen map[int]bool) bool
	persistent = func(i int, seen map[int]bool) bool {
		re := raws[i]
		if re.persistable != nil {
			return *re.persistable
		}
		if re.generalizes == "" || seen[i] {
			return true
		}
		seen[i] = true
		if j, ok := byName[re.generalizes]; ok {
			return persistent(j, seen)
		}
		if p, known := meta.SystemEntityPersistable(re.generalizes); known {
			return p
		}
		return true // an unknown parent: assume a table, so a change is reported, not hidden
	}
	for i := range raws {
		m.Entities[i].Stored = !raws[i].external && persistent(i, map[int]bool{})
	}

	for _, ra := range assocs {
		a := ra.a
		if i, ok := byID[ra.from]; ok {
			a.From, a.FromName = m.Entities[i].GUID, m.Entities[i].Name
		}
		to := -1
		if i, ok := byID[ra.to]; ok {
			to = i
		} else if i, ok := byName[ra.toName]; ok {
			to = i
		}
		if to >= 0 {
			a.To, a.ToName = m.Entities[to].GUID, m.Entities[to].Name
		} else if ra.toName != "" {
			// A System entity: stored, and the same in every model.
			a.To, a.ToName = "system:"+ra.toName, ra.toName
			if p, known := meta.SystemEntityPersistable(ra.toName); known && p {
				m.Entities = append(m.Entities, projectreport.RelEntity{GUID: a.To, Name: ra.toName, Module: "System", Stored: true})
			}
		}
		m.Associations = append(m.Associations, a)
	}
	m.Entities = dedupeSystem(m.Entities)
	return m, nil
}

func assocOf(av bson.D, module string) projectreport.RelAssoc {
	return projectreport.RelAssoc{
		GUID: binHex(field(av, "GUID")), Name: module + "." + str(field(av, "Name")), Module: module,
		Type: str(field(av, "Type")), StorageFormat: str(field(av, "StorageFormat")),
	}
}

// dedupeSystem keeps one placeholder per System entity an association names.
func dedupeSystem(es []projectreport.RelEntity) []projectreport.RelEntity {
	seen := map[string]bool{}
	out := es[:0]
	for _, e := range es {
		if strings.HasPrefix(e.GUID, "system:") {
			if seen[e.GUID] {
				continue
			}
			seen[e.GUID] = true
		}
		out = append(out, e)
	}
	return out
}

func field(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// elements returns the documents of a stored list, whose first item is the
// list-type marker.
func elements(v any) []bson.D {
	a, _ := v.(bson.A)
	var out []bson.D
	for _, x := range a {
		if d, ok := x.(bson.D); ok {
			out = append(out, d)
		}
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func binHex(v any) string {
	switch b := v.(type) {
	case bson.Binary:
		return hex.EncodeToString(b.Data)
	case []byte:
		return hex.EncodeToString(b)
	}
	return ""
}

func bsonInt(v any) (int, bool) {
	switch n := v.(type) {
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// resolveReleaseBaseline turns --against into the path of the deployed model:
// a .mpr on disk as given, or a git revision of the project, exported to a
// temporary directory. cleanup is never nil.
func resolveReleaseBaseline(projectPath, against string) (mprPath, label string, cleanup func(), err error) {
	cleanup = func() {}
	if fi, statErr := os.Stat(against); statErr == nil && !fi.IsDir() {
		return against, against, cleanup, nil
	}
	if strings.HasSuffix(strings.ToLower(against), ".mpr") {
		return "", "", cleanup, fmt.Errorf("--against %s: no such file", against)
	}
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return "", "", cleanup, err
	}
	dir := filepath.Dir(abs)
	top, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", cleanup, fmt.Errorf("--against %s is neither a .mpr file nor a git revision of %s: %v", against, dir, err)
	}
	top = strings.TrimSpace(top)
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	if real, err := filepath.EvalSymlinks(top); err == nil {
		top = real
	}
	rel, err := filepath.Rel(top, dir)
	if err != nil {
		return "", "", cleanup, err
	}
	commit, err := gitOutput(top, "rev-parse", "--verify", "--quiet", against+"^{commit}")
	if err != nil {
		return "", "", cleanup, fmt.Errorf("--against %s: not a .mpr file, and not a revision in %s", against, top)
	}
	commit = strings.TrimSpace(commit)
	mprRel := filepath.ToSlash(filepath.Join(rel, filepath.Base(abs)))
	paths := []string{mprRel}
	contents := filepath.ToSlash(filepath.Join(rel, "mprcontents"))
	if out, err := gitOutput(top, "ls-tree", "-d", "--name-only", commit, contents); err == nil && strings.TrimSpace(out) != "" {
		paths = append(paths, contents)
	}
	if out, err := gitOutput(top, "ls-tree", "--name-only", commit, mprRel); err != nil || strings.TrimSpace(out) == "" {
		return "", "", cleanup, fmt.Errorf("--against %s: %s is not in that revision", against, mprRel)
	}
	tmp, err := os.MkdirTemp("", "mxcli-release-*")
	if err != nil {
		return "", "", cleanup, err
	}
	cleanup = func() { os.RemoveAll(tmp) }
	cmd := exec.Command("git", append([]string{"-C", top, "archive", "--format=tar", commit, "--"}, paths...)...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("git archive %s: %v: %s", against, err, strings.TrimSpace(errOut.String()))
	}
	if err := untar(&out, tmp); err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	short := commit
	if len(short) > 12 {
		short = short[:12]
	}
	return filepath.Join(tmp, filepath.FromSlash(mprRel)), fmt.Sprintf("git %s (%s)", against, short), cleanup, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// untar writes a git archive into dir. Paths are checked to stay inside it.
func untar(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(h.Name))
		if !strings.HasPrefix(target, filepath.Clean(dir)+string(os.PathSeparator)) {
			return fmt.Errorf("git archive entry %q leaves the target directory", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
}
