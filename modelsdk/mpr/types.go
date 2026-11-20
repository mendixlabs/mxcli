package mpr

import (
	"fmt"
)

// UnitInfo contains basic information about a unit.
type UnitInfo struct {
	ID              string
	ContainerID     string
	ContainmentName string
	Type            string
}

// FolderInfo contains information about a project folder.
type FolderInfo struct {
	ID          string
	ContainerID string
	Name        string
}

// ModuleInfo contains basic information about a module.
type ModuleInfo struct {
	ID   string
	Name string
}

// ListModules returns all modules in the project.
func (r *Reader) ListModules() ([]*ModuleInfo, error) {
	// A module's ID and name, which is all a ModuleInfo holds, are in the unit
	// index. This used to read every module's contents and unmarshal the whole
	// document for the one field, and it is called to rebuild the container
	// hierarchy after every document a script creates (ako/mxcli#2160).
	headers, err := r.ListUnitHeaders("Projects$ModuleImpl")
	if err != nil {
		return nil, err
	}
	modules := make([]*ModuleInfo, 0, len(headers)+1)
	for _, h := range headers {
		modules = append(modules, &ModuleInfo{ID: h.ID, Name: h.Name})
	}
	modules = append(modules, buildSystemModuleInfo())
	return modules, nil
}

// GetModule retrieves a module by ID.
func (r *Reader) GetModule(id string) (*ModuleInfo, error) {
	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}
	for _, m := range modules {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, fmt.Errorf("module not found: %s", id)
}

// GetModuleByName retrieves a module by name.
func (r *Reader) GetModuleByName(name string) (*ModuleInfo, error) {
	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}
	for _, m := range modules {
		if m.Name == name {
			return m, nil
		}
	}
	return nil, fmt.Errorf("module not found: %s", name)
}

// ListUnits returns all units with their IDs and types.
func (r *Reader) ListUnits() ([]*UnitInfo, error) {
	units, err := r.listUnitTypes()
	if err != nil {
		return nil, err
	}
	var result []*UnitInfo
	for _, u := range units {
		result = append(result, &UnitInfo{
			ID:              u.ID,
			ContainerID:     u.ContainerID,
			ContainmentName: u.ContainmentName,
			Type:            u.Type,
		})
	}
	return result, nil
}

// ListFolders returns all project folders with their names.
func (r *Reader) ListFolders() ([]*FolderInfo, error) {
	units, err := r.ListUnitHeaders("Projects$Folder")
	if err != nil {
		return nil, err
	}
	result := make([]*FolderInfo, 0, len(units))
	for _, u := range units {
		result = append(result, &FolderInfo{
			ID:          u.ID,
			ContainerID: u.ContainerID,
			Name:        u.Name,
		})
	}
	return result, nil
}

// resolveContents resolves unit contents, loading from external file for MPR v2.
// For v2 format, contents are stored in mprcontents/XX/YY/UUID.mxunit files;
// the inline contents parameter is empty (the Unit table has no Contents column).
func (r *Reader) resolveContents(unitID string, contents []byte) ([]byte, error) {
	if r.version == MPRVersionV1 {
		return contents, nil
	}
	// Already loaded (e.g. by readMprContents in listUnitsByTypeV2).
	if len(contents) >= 4 {
		return contents, nil
	}
	// V2: load from mprcontents/<XX>/<YY>/<UUID>.mxunit
	// unitID is in the swapped-UUID format used by blobToUUID / blobToUUIDSwapped.
	return r.readMprContents(unitID)
}
