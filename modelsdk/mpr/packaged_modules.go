// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"archive/zip"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/internal/zipentry"
)

// Packaged modules (ako/mxcli#1780, ako/mxcli#1998).
//
// A project can hold a module as a file instead of as units: Studio Pro keeps
// packaged and sealed add-on modules as modules/<Name>.mxmodule beside the
// .mpr, in both MPR v1 and v2, and the .mpr has no unit for them — measured on
// EvoraICX (11.12.2, v1: 30 module units, 3 packages, 34 modules in Studio
// Pro) and FactoryManagement (11.16.0, v2: 4 packages). A reader of the units
// alone did not see them, so `show modules` left them out, `describe` said
// "not found", and `check --references` refused valid calls into them.
//
// The package is a zip holding package.xml (<module name="…"/>,
// <projectFile path="…"/>) and that project file: an ordinary v1 .mpr, or the
// single-BSON module tree of a protected add-on (module_tree.go). Its module's
// units are listed with the project's own, the module unit attached to the
// project root, so every reader sees the module as Studio Pro shows it.
//
// They are read-only. Nothing in the .mpr holds them, so a write would update
// a unit that is not stored or put a new one under a container the .mpr does
// not have: the writer refuses both (PackagedUnitError). Whether and how Studio
// Pro lets such a module be changed is not measured, and the GUID rules in
// CLAUDE.md forbid guessing at it.

// PackagedModule is a module the project holds as a package file.
type PackagedModule struct {
	// Name is the module's name, from package.xml.
	Name string
	// File is the package's path relative to the project directory, with
	// forward slashes: modules/<Name>.mxmodule.
	File string
	// ModuleID is the module unit's ID; empty when Err is set.
	ModuleID string
	// Err says why the package's model could not be read. The module is then
	// not listed: there is no unit to list it by.
	Err error
}

// packagedSet is what the project's package files hold, read once per reader.
type packagedSet struct {
	modules []PackagedModule
	units   []rawUnit
	byID    map[string]int // unit ID → index in units
	module  map[string]int // unit ID → index in modules of the package holding it
}

// packagedModulesDir is where Studio Pro keeps module packages, relative to the
// project directory.
const packagedModulesDir = "modules"

// packagedModuleExt is the extension of a module package in that folder.
const packagedModuleExt = ".mxmodule"

// PackagedModules returns the modules the project holds as package files,
// sorted by name, including those whose model could not be read (Err set).
func (r *Reader) PackagedModules() []PackagedModule {
	out := append([]PackagedModule(nil), r.packagedUnits().modules...)
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// PackagedModuleOf returns the package holding the unit whose ID is id, and
// false when the project stores that unit itself.
func (r *Reader) PackagedModuleOf(id string) (PackagedModule, bool) {
	p := r.packagedUnits()
	if _, ok := p.byID[id]; !ok {
		return PackagedModule{}, false
	}
	return p.modules[p.module[id]], true
}

// PackagedUnitError refuses a write to a unit a module package holds, or into
// one as its container, naming the package. nil for any other unit.
func (r *Reader) PackagedUnitError(unitID, containerID string) error {
	for _, id := range []string{unitID, containerID} {
		if id == "" {
			continue
		}
		if pm, ok := r.PackagedModuleOf(id); ok {
			return fmt.Errorf("module %s is a packaged module (%s): it is read-only, as nothing in the project file "+
				"holds its documents; change it in the package's source project and re-import the package", pm.Name, pm.File)
		}
	}
	return nil
}

// packagedUnits reads the project's package files on first use.
func (r *Reader) packagedUnits() *packagedSet {
	r.packagedOnce.Do(func() {
		r.packaged = r.loadPackagedModules()
	})
	return r.packaged
}

func (r *Reader) loadPackagedModules() *packagedSet {
	set := &packagedSet{byID: map[string]int{}, module: map[string]int{}}
	if r.path == "" {
		return set
	}
	projectDir := filepath.Dir(r.path)
	files, _ := filepath.Glob(filepath.Join(projectDir, packagedModulesDir, "*"+packagedModuleExt))
	if len(files) == 0 {
		return set
	}
	sort.Strings(files)
	rootID, err := r.GetProjectRootID()
	if err != nil {
		return set
	}
	stored, err := r.listStoredUnitsByType("Projects$ModuleImpl")
	if err != nil {
		return set
	}
	storedNames := map[string]bool{}
	for _, u := range stored {
		storedNames[strings.ToLower(unitIndexEntryOf(u.Contents).Name)] = true
	}
	storedIDs := map[string]bool{}
	if ids, err := r.ListAllUnitIDs(); err == nil {
		for _, id := range ids {
			storedIDs[id] = true
		}
	}

	for _, file := range files {
		rel := packagedModulesDir + "/" + filepath.Base(file)
		pm := PackagedModule{File: rel, Name: strings.TrimSuffix(filepath.Base(file), packagedModuleExt)}
		units, name, err := readPackagedModule(file, pm.Name)
		if name != "" {
			pm.Name = name
		}
		switch {
		case err != nil:
			pm.Err = err
		case storedNames[strings.ToLower(pm.Name)]:
			// The project stores a module of this name: that is the one every
			// reader and writer works on. The package is left out rather than
			// listed twice.
			continue
		default:
			for _, u := range units {
				if _, dup := set.byID[u.ID]; dup || storedIDs[u.ID] {
					pm.Err = fmt.Errorf("unit %s is also in the project or another package", u.ID)
					break
				}
			}
		}
		if pm.Err != nil {
			set.modules = append(set.modules, pm)
			continue
		}
		pm.ModuleID = units[0].ID
		units[0].ContainerID = rootID
		units[0].ContainmentName = "Modules"
		mi := len(set.modules)
		set.modules = append(set.modules, pm)
		for _, u := range units {
			set.byID[u.ID] = len(set.units)
			set.module[u.ID] = mi
			set.units = append(set.units, u)
		}
	}
	return set
}

// readPackagedModule reads a module package's module as units, module unit
// first, and the module name package.xml declares.
func readPackagedModule(path, fallbackName string) ([]rawUnit, string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, "", fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer zr.Close()
	manifest, err := zipFile(&zr.Reader, "package.xml")
	if err != nil {
		return nil, "", err
	}
	name, projectFile := parsePackageXML(manifest)
	if name == "" {
		name = fallbackName
	}
	if projectFile == "" {
		projectFile = "project.mpr"
	}
	data, err := zipFile(&zr.Reader, projectFile)
	if err != nil {
		return nil, name, err
	}
	var units []rawUnit
	switch {
	case IsSQLite(data):
		units, err = sqliteModuleUnits(data, name)
	default:
		if _, ok := ModuleTreeVersion(data); !ok {
			return nil, name, fmt.Errorf("%s in %s is neither a .mpr nor a module tree", projectFile, filepath.Base(path))
		}
		var split []TreeUnit
		split, err = ModuleTreeUnits(data, name)
		for _, u := range split {
			units = append(units, rawUnit{ID: u.ID, ContainerID: u.ContainerID, ContainmentName: u.ContainmentName, Type: u.Type, Contents: u.Contents})
		}
	}
	if err != nil {
		return nil, name, err
	}
	if len(units) == 0 || units[0].Type != "Projects$ModuleImpl" {
		return nil, name, fmt.Errorf("no module %s in %s", name, filepath.Base(path))
	}
	return units, name, nil
}

// parsePackageXML returns the module name and project file a package.xml
// declares. The elements are matched by local name wherever they sit: a .mpk
// wraps <modelerProject> in <package>, and namespaces vary.
func parsePackageXML(data []byte) (module, projectFile string) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return module, projectFile
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, a := range se.Attr {
			switch {
			case se.Name.Local == "module" && a.Name.Local == "name" && module == "":
				module = a.Value
			case se.Name.Local == "projectFile" && a.Name.Local == "path" && projectFile == "":
				projectFile = a.Value
			}
		}
	}
}

func zipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := zipentry.Open(f)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("no %s in the package", name)
}

// sqliteModuleUnits reads the named module of a v1 .mpr held in memory: the
// module unit, then every unit under it, parents before children.
func sqliteModuleUnits(data []byte, moduleName string) ([]rawUnit, error) {
	tmp, err := os.CreateTemp("", "mxcli-mxmodule-*.mpr")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+tmp.Name()+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT UnitID, ContainerID, ContainmentName, Contents FROM Unit`)
	if err != nil {
		return nil, fmt.Errorf("read the package's project: %w", err)
	}
	defer rows.Close()
	var all []rawUnit
	for rows.Next() {
		var id, container []byte
		var u rawUnit
		if err := rows.Scan(&id, &container, &u.ContainmentName, &u.Contents); err != nil {
			return nil, fmt.Errorf("read the package's project: %w", err)
		}
		u.ID, u.ContainerID = blobToUUID(id), blobToUUID(container)
		u.Type = getTypeFromContents(u.Contents)
		all = append(all, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	children := map[string][]int{}
	moduleAt := -1
	for i, u := range all {
		if u.ID != u.ContainerID {
			children[u.ContainerID] = append(children[u.ContainerID], i)
		}
		if u.Type == "Projects$ModuleImpl" && strings.EqualFold(unitIndexEntryOf(u.Contents).Name, moduleName) {
			moduleAt = i
		}
	}
	if moduleAt < 0 {
		return nil, errors.New("module " + moduleName + " not found in the package's project")
	}
	var out []rawUnit
	var walk func(i int)
	walk = func(i int) {
		out = append(out, all[i])
		for _, c := range children[all[i].ID] {
			walk(c)
		}
	}
	walk(moduleAt)
	return out, nil
}

// packagedUnitsOf returns the packaged units of exactly typeName ("" for all)
// that keep accepts (nil keeps all) and, when containerID is not empty, whose
// container it is.
func (r *Reader) packagedUnitsOf(typeName, containerID string, keep func(id string) bool) []rawUnit {
	var out []rawUnit
	for _, u := range r.packagedUnits().units {
		if (typeName == "" || u.Type == typeName) &&
			(containerID == "" || u.ContainerID == containerID) &&
			(keep == nil || keep(u.ID)) {
			out = append(out, u)
		}
	}
	return out
}

// packagedUnit returns the packaged unit whose ID is id.
func (r *Reader) packagedUnit(id string) (rawUnit, bool) {
	p := r.packagedUnits()
	i, ok := p.byID[id]
	if !ok {
		return rawUnit{}, false
	}
	return p.units[i], true
}
