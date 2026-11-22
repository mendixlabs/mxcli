// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Association storage formats, spelled as the model spells them
// (domainmodel.StorageFormatColumn / StorageFormatTable). These are the only
// two values AssocInfo.StorageFormat may hold.
const (
	StorageFormatColumn = "Column" // FK column on the owner entity's table
	StorageFormatTable  = "Table"  // separate junction table
)

// AssocInfo holds resolved association metadata for an import LINK mapping.
type AssocInfo struct {
	// From the LINK clause
	SourceColumn string // source query column name
	LookupAttr   string // child entity attribute for lookup (empty = direct ID)

	// Resolved from MPR + system tables
	AssociationName string // qualified name (e.g., "HR.Employee_Department")
	ChildEntity     string // qualified name of child entity
	StorageFormat   string // StorageFormatColumn or StorageFormatTable

	// Database names (from mendixsystem$association or conventions)
	FKColumnName  string // Column storage: FK column in parent table
	JunctionTable string // Table storage: junction table name
	ParentColName string // Table storage: parent ID column in junction table
	ChildColName  string // Table storage: child ID column in junction table

	// Lookup cache: value → Mendix object ID
	LookupCache map[string]int64
}

// Lookup resolves a source value to a Mendix object ID using the pre-built cache.
// Returns (id, true) if found, (0, false) if not found.
// If LookupAttr is empty (direct mode), the value is expected to be an int64 already.
func (a *AssocInfo) Lookup(value any) (int64, bool) {
	if value == nil {
		return 0, false
	}
	if a.LookupAttr == "" {
		// Direct ID mode — try to interpret as int64
		switch v := value.(type) {
		case int64:
			return v, true
		case int32:
			return int64(v), true
		case float64:
			return int64(v), true
		case int:
			return int64(v), true
		default:
			return 0, false
		}
	}
	// Lookup mode — convert value to string key
	key := fmt.Sprintf("%v", value)
	id, ok := a.LookupCache[key]
	return id, ok
}

// AssocSystemInfo holds a row from mendixsystem$association.
type AssocSystemInfo struct {
	AssociationName string
	TableName       string
	ChildColumnName string
	// ParentColumnName is the junction table's column for the parent (FROM)
	// entity; for column storage Mendix writes `id` here (ako/mxcli#1686).
	ParentColumnName string
	StorageFormat    string // decoded: StorageFormatColumn or StorageFormatTable
}

// decodeStorageFormat normalises a mendixsystem$association.storage_format
// value to StorageFormatColumn or StorageFormatTable.
//
// The runtime stores a smallint: 1 for a foreign-key column on the owner's
// table, 2 for a junction table — measured on Mendix 10.24.24, 11.12.2, 11.13.0 and
// 11.14.0 (ako/mxcli#1454). The decimal text of those numbers and the model's
// own names are accepted as well, so a driver that returns the column as text,
// or a value already in model form, decodes the same way. Anything else, NULL
// included, is an error: a guess would write the link to the wrong place or —
// as the string comparison this replaces did — nowhere at all, while the
// import reported success.
func decodeStorageFormat(raw any) (string, error) {
	var n int64
	switch v := raw.(type) {
	case nil:
		return "", fmt.Errorf("storage_format is NULL")
	case int64:
		n = v
	case int32:
		n = int64(v)
	case int16:
		n = int64(v)
	case int:
		n = int64(v)
	case []byte:
		return decodeStorageFormatText(string(v))
	case string:
		return decodeStorageFormatText(v)
	default:
		return "", fmt.Errorf("unrecognised storage_format %v (%T)", raw, raw)
	}
	switch n {
	case 1:
		return StorageFormatColumn, nil
	case 2:
		return StorageFormatTable, nil
	}
	return "", fmt.Errorf("unrecognised storage_format %d (expected 1 = column, 2 = junction table)", n)
}

func decodeStorageFormatText(s string) (string, error) {
	switch t := strings.TrimSpace(s); {
	case t == "1" || strings.EqualFold(t, StorageFormatColumn):
		return StorageFormatColumn, nil
	case t == "2" || strings.EqualFold(t, StorageFormatTable):
		return StorageFormatTable, nil
	}
	return "", fmt.Errorf("unrecognised storage_format %q (expected 1 = column, 2 = junction table)", s)
}

// LookupAssociationInfo queries mendixsystem$association for the given association name.
// It returns (nil, nil) when the association has no row (the app has not been
// started since it was created), and an error when the row's storage format
// cannot be decoded.
func LookupAssociationInfo(ctx context.Context, conn *Connection, assocName string) (*AssocSystemInfo, error) {
	query := `
		SELECT association_name, table_name, parent_column_name, child_column_name, storage_format
		FROM mendixsystem$association
		WHERE association_name = $1`

	var info AssocSystemInfo
	var parentCol, childCol sql.NullString
	var rawFormat any
	err := conn.DB.QueryRowContext(ctx, query, assocName).Scan(
		&info.AssociationName, &info.TableName, &parentCol, &childCol, &rawFormat)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // not found — caller will fall back to conventions
		}
		return nil, fmt.Errorf("failed to query mendixsystem$association: %w", err)
	}
	format, err := decodeStorageFormat(rawFormat)
	if err != nil {
		return nil, fmt.Errorf("association %s in mendixsystem$association: %w; "+
			"cannot tell whether it is stored as a column or a junction table, so nothing would be linked", assocName, err)
	}
	info.StorageFormat = format
	info.ChildColumnName = childCol.String
	info.ParentColumnName = parentCol.String
	return &info, nil
}

// BuildLookupCache queries the child entity table to build a value → Mendix ID mapping.
func BuildLookupCache(ctx context.Context, conn *Connection, childTable, lookupColumn string) (map[string]int64, error) {
	query := fmt.Sprintf(`SELECT id, %q FROM %q`, lookupColumn, childTable)

	rows, err := conn.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query child table %q for lookup cache: %w", childTable, err)
	}
	defer rows.Close()

	cache := make(map[string]int64)
	for rows.Next() {
		var id int64
		var val any
		if err := rows.Scan(&id, &val); err != nil {
			return nil, fmt.Errorf("failed to scan lookup row: %w", err)
		}
		key := fmt.Sprintf("%v", val)
		if _, exists := cache[key]; exists {
			return nil, fmt.Errorf("ambiguous lookup: multiple rows in %q have %s = %q", childTable, lookupColumn, key)
		}
		cache[key] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lookup cache iteration error: %w", err)
	}

	return cache, nil
}

// AssocColumnNameFromConvention derives the FK column name from Mendix naming conventions.
// Convention: {module}${association_name_lower} — all lowercase with $ separator.
func AssocColumnNameFromConvention(assocQualifiedName string) string {
	parts := strings.SplitN(assocQualifiedName, ".", 2)
	if len(parts) != 2 {
		return strings.ToLower(assocQualifiedName)
	}
	return strings.ToLower(parts[0]) + "$" + strings.ToLower(parts[1])
}

// JunctionTableFromConvention derives the junction table name from Mendix naming conventions.
// Convention: same as column name — {module}${association_name_lower}.
func JunctionTableFromConvention(assocQualifiedName string) string {
	return AssocColumnNameFromConvention(assocQualifiedName)
}

// JunctionColumnFromConvention derives a junction table column name.
// Convention: {module}${entity_name_lower}id
// JunctionColumns names a junction table's parent and child columns: the
// ones mendixsystem$association records when it has the row, else the naming
// convention. A self-association's two ends are one entity, so the convention
// numbers them — measured on Mendix 11.12.6, Person_Friend from Person to
// Person is stored as myfirstmodule$personid1 (parent) and
// myfirstmodule$personid2 (child). Naming both after the entity put one
// column in the insert twice (ako/mxcli#1686).
func JunctionColumns(sys *AssocSystemInfo, parentEntity, childEntity string) (parent, child string) {
	if sys != nil && sys.StorageFormat == StorageFormatTable && sys.ParentColumnName != "" && sys.ChildColumnName != "" {
		return sys.ParentColumnName, sys.ChildColumnName
	}
	parent = JunctionColumnFromConvention(parentEntity)
	child = JunctionColumnFromConvention(childEntity)
	if strings.EqualFold(parentEntity, childEntity) {
		return parent + "1", child + "2"
	}
	return parent, child
}

func JunctionColumnFromConvention(entityQualifiedName string) string {
	parts := strings.SplitN(entityQualifiedName, ".", 2)
	if len(parts) != 2 {
		return strings.ToLower(entityQualifiedName) + "id"
	}
	return strings.ToLower(parts[0]) + "$" + strings.ToLower(parts[1]) + "id"
}
