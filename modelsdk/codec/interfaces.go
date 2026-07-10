package codec

import (
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// UnitReader provides read-only access to MPR units.
type UnitReader interface {
	ListUnits() []UnitInfo
	LoadUnit(id element.ID) (bson.Raw, error)
	Path() string
	IsWritable() bool
}

// UnitWriter provides write access to MPR units.
type UnitWriter interface {
	InsertUnit(unitID, containerID, containmentName, unitType string, contents []byte) error
	DeleteUnit(unitID string) error
	DeleteChildUnits(parentID string) error
	SaveUnit(id element.ID, data []byte) error
	FlushUnits(units map[element.ID][]byte) error
}

// Compile-time interface satisfaction checks.
var (
	_ UnitReader = (*Store)(nil)
	_ UnitWriter = (*Store)(nil)
)
