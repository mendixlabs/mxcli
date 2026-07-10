package codec

import (
	"fmt"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr/version"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// UnitInfo holds basic metadata for a unit in the MPR store.
type UnitInfo struct {
	ID          element.ID
	ContainerID element.ID
	Type        string // BSON $Type (e.g. "Microflows$Microflow")
	Name        string // BSON Name field (e.g. "MyModule" for modules)
}

// Store wraps mpr.Reader (read-only) or mpr.Writer (read-write) and exposes
// unit-level BSON access for the codec layer.
type Store struct {
	reader   *mpr.Reader
	writer   *mpr.Writer // nil for read-only
	writable bool
}

// Open opens the MPR file at path for reading.
func Open(path string) (*Store, error) {
	r, err := mpr.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open MPR: %w", err)
	}
	return &Store{reader: r}, nil
}

// OpenForWriting opens the MPR file at path for reading and writing.
func OpenForWriting(path string) (*Store, error) {
	w, err := mpr.NewWriter(path)
	if err != nil {
		return nil, fmt.Errorf("open MPR for writing: %w", err)
	}
	return &Store{
		reader:   w.ConcreteReader(),
		writer:   w,
		writable: true,
	}, nil
}

// Close releases resources held by the underlying Reader/Writer.
func (s *Store) Close() error {
	if s.writer != nil {
		return s.writer.Close()
	}
	return s.reader.Close()
}

// IsWritable returns true if the store was opened for writing.
func (s *Store) IsWritable() bool { return s.writable }

// ListUnits returns metadata for every unit in the project.
func (s *Store) ListUnits() []UnitInfo {
	listStart := time.Now()
	refs, err := s.reader.ListUnitsByType("")
	if err != nil {
		return nil
	}
	units := make([]UnitInfo, 0, len(refs))
	for _, ref := range refs {
		name := ""
		// Use Contents from ListUnitsByType when available to avoid N+1 re-reads.
		raw := ref.Contents
		if len(raw) == 0 {
			raw, _ = s.reader.GetRawUnitBytes(ref.ID)
		}
		if len(raw) > 0 {
			if val, err := bson.Raw(raw).LookupErr("Name"); err == nil {
				name, _ = val.StringValueOK()
			}
		}
		units = append(units, UnitInfo{
			ID:          element.ID(ref.ID),
			ContainerID: element.ID(ref.ContainerID),
			Type:        ref.Type,
			Name:        name,
		})
	}
	_ = listStart // reserved for future debug logging
	return units
}

// LoadUnit fetches the raw BSON document for the unit identified by id.
func (s *Store) LoadUnit(id element.ID) (bson.Raw, error) {
	// Use GetRawUnitBytes to get the original BSON bytes directly.
	// Do NOT use GetRawUnit (returns map[string]any) + bson.Marshal —
	// that round-trip corrupts Binary $ID fields into strings.
	b, err := s.reader.GetRawUnitBytes(string(id))
	if err != nil {
		return nil, fmt.Errorf("load unit %s: %w", id, err)
	}
	return bson.Raw(b), nil
}

// SaveUnit writes a unit's BSON bytes back to the MPR.
// Must have been opened with OpenForWriting.
func (s *Store) SaveUnit(id element.ID, data []byte) error {
	if !s.writable {
		return fmt.Errorf("store is read-only — use OpenForWriting")
	}
	wt, err := s.writer.BeginWriteTransaction()
	if err != nil {
		return fmt.Errorf("begin write transaction: %w", err)
	}
	if err := wt.WriteUnit(string(id), data); err != nil {
		wt.Rollback()
		return fmt.Errorf("write unit %s: %w", id, err)
	}
	if err := wt.Commit(); err != nil {
		return fmt.Errorf("commit unit %s: %w", id, err)
	}
	return nil
}

// Path returns the file path of the underlying MPR.
func (s *Store) Path() string { return s.reader.Path() }

// GetProjectRootID returns the ID of the project root unit.
func (s *Store) GetProjectRootID() (string, error) {
	return s.reader.GetProjectRootID()
}

// GetModuleByName retrieves a module by name from the underlying MPR reader.
func (s *Store) GetModuleByName(name string) (*mpr.ModuleInfo, error) {
	return s.reader.GetModuleByName(name)
}

// UpdateUnitContainer changes the container (parent) of a unit.
// Must have been opened with OpenForWriting.
func (s *Store) UpdateUnitContainer(unitID, newContainerID string) error {
	if !s.writable {
		return fmt.Errorf("store is read-only — use OpenForWriting")
	}
	return s.writer.UpdateUnitContainer(unitID, newContainerID)
}

// InvalidateReaderCache clears the underlying reader's cached data.
func (s *Store) InvalidateReaderCache() {
	s.reader.InvalidateCache()
}

// ProjectVersion returns the Mendix version of the project.
func (s *Store) ProjectVersion() *version.ProjectVersion {
	return s.reader.ProjectVersion()
}

// DeleteChildUnits recursively deletes all units whose ContainerID matches parentID.
func (s *Store) DeleteChildUnits(parentID string) error {
	if !s.writable {
		return fmt.Errorf("store is read-only — use OpenForWriting")
	}
	return s.writer.DeleteChildUnits(parentID)
}

// InsertUnit creates a new unit in the MPR.
func (s *Store) InsertUnit(unitID, containerID, containmentName, unitType string, contents []byte) error {
	if !s.writable {
		return fmt.Errorf("store is read-only — use OpenForWriting")
	}
	return s.writer.InsertUnit(unitID, containerID, containmentName, unitType, contents)
}

// DeleteUnit removes a unit from the MPR.
func (s *Store) DeleteUnit(unitID string) error {
	if !s.writable {
		return fmt.Errorf("store is read-only — use OpenForWriting")
	}
	return s.writer.DeleteUnit(unitID)
}

// FlushUnits saves multiple units atomically in a single transaction.
func (s *Store) FlushUnits(units map[element.ID][]byte) error {
	if !s.writable {
		return fmt.Errorf("store is read-only — use OpenForWriting")
	}
	wt, err := s.writer.BeginWriteTransaction()
	if err != nil {
		return fmt.Errorf("begin write transaction: %w", err)
	}
	for id, data := range units {
		if err := wt.WriteUnit(string(id), data); err != nil {
			wt.Rollback()
			return fmt.Errorf("write unit %s: %w", id, err)
		}
	}
	if err := wt.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
