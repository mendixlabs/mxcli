// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"io"
	"log"
	"path/filepath"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"go.mongodb.org/mongo-driver/bson"
	_ "modernc.org/sqlite"
)

// =============================================================================
// removeRolesFromAccessRule — unit tests for multi-role handling
// =============================================================================

func makeAccessRule(roleNames ...string) bson.D {
	roles := bson.A{int32(1)} // Mendix array marker
	for _, r := range roleNames {
		roles = append(roles, r)
	}
	return bson.D{
		{Key: "$Type", Value: "DomainModels$AccessRule"},
		{Key: "AllowedModuleRoles", Value: roles},
		{Key: "AllowCreate", Value: true},
	}
}

func getRoleNames(rule bson.D) []string {
	for _, f := range rule {
		if f.Key != "AllowedModuleRoles" {
			continue
		}
		arr, ok := f.Value.(bson.A)
		if !ok {
			return nil
		}
		var names []string
		for _, item := range arr {
			if s, ok := item.(string); ok {
				names = append(names, s)
			}
		}
		return names
	}
	return nil
}

func TestRemoveRolesFromAccessRule_SingleRole_ExactMatch(t *testing.T) {
	rule := makeAccessRule("Mod.RoleA")
	keep, modified := removeRolesFromAccessRule(rule, map[string]bool{"Mod.RoleA": true})
	if keep {
		t.Error("expected rule to be deleted (no roles remaining)")
	}
	if !modified {
		t.Error("expected modified=true")
	}
}

func TestRemoveRolesFromAccessRule_MultiRole_RemoveOne(t *testing.T) {
	rule := makeAccessRule("Mod.User", "Mod.Admin")
	keep, modified := removeRolesFromAccessRule(rule, map[string]bool{"Mod.User": true})
	if !keep {
		t.Error("expected rule to be kept (Admin still present)")
	}
	if !modified {
		t.Error("expected modified=true")
	}
	names := getRoleNames(rule)
	if len(names) != 1 || names[0] != "Mod.Admin" {
		t.Errorf("expected [Mod.Admin], got %v", names)
	}
}

func TestRemoveRolesFromAccessRule_MultiRole_RemoveAll(t *testing.T) {
	rule := makeAccessRule("Mod.User", "Mod.Admin")
	keep, modified := removeRolesFromAccessRule(rule, map[string]bool{"Mod.User": true, "Mod.Admin": true})
	if keep {
		t.Error("expected rule to be deleted (no roles remaining)")
	}
	if !modified {
		t.Error("expected modified=true")
	}
}

func TestRemoveRolesFromAccessRule_NoMatch(t *testing.T) {
	rule := makeAccessRule("Mod.User", "Mod.Admin")
	keep, modified := removeRolesFromAccessRule(rule, map[string]bool{"Mod.Other": true})
	if !keep {
		t.Error("expected rule to be kept")
	}
	if modified {
		t.Error("expected modified=false")
	}
	names := getRoleNames(rule)
	if len(names) != 2 {
		t.Errorf("expected 2 roles unchanged, got %v", names)
	}
}

func TestRemoveRolesFromAccessRule_NoAllowedModuleRoles(t *testing.T) {
	rule := bson.D{
		{Key: "$Type", Value: "DomainModels$AccessRule"},
		{Key: "AllowCreate", Value: true},
	}
	keep, modified := removeRolesFromAccessRule(rule, map[string]bool{"Mod.User": true})
	if !keep {
		t.Error("expected rule to be kept (no AllowedModuleRoles field)")
	}
	if modified {
		t.Error("expected modified=false")
	}
}

func TestRemoveRolesFromAccessRule_ThreeRoles_RemoveMiddle(t *testing.T) {
	rule := makeAccessRule("Mod.A", "Mod.B", "Mod.C")
	keep, modified := removeRolesFromAccessRule(rule, map[string]bool{"Mod.B": true})
	if !keep {
		t.Error("expected rule to be kept")
	}
	if !modified {
		t.Error("expected modified=true")
	}
	names := getRoleNames(rule)
	if len(names) != 2 || names[0] != "Mod.A" || names[1] != "Mod.C" {
		t.Errorf("expected [Mod.A, Mod.C], got %v", names)
	}
}

// =============================================================================
// mergeAccessRule — malformed BSON must not panic
// =============================================================================

// Not parallel-safe: redirects global log output.
func TestMergeAccessRule_UnexpectedTypes_NoPanic(t *testing.T) {
	origOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(origOutput)

	existing := bson.D{
		{Key: "$Type", Value: "DomainModels$AccessRule"},
		{Key: "AllowCreate", Value: 42},           // wrong type: int instead of bool
		{Key: "AllowDelete", Value: "not-a-bool"}, // wrong type: string instead of bool
		{Key: "DefaultMemberAccessRights", Value: 99},
	}
	newRule := bson.D{
		{Key: "$Type", Value: "DomainModels$AccessRule"},
		{Key: "AllowCreate", Value: true},
		{Key: "AllowDelete", Value: false},
		{Key: "DefaultMemberAccessRights", Value: "ReadWrite"},
	}

	// Must not panic
	result := mergeAccessRule(existing, newRule)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

// =============================================================================
// AddEntityAccessRule — XPath constraint preserved and rights readable (#431)
// =============================================================================

// newTestWriterSecurity creates an in-memory SQLite writer for security tests.
func newTestWriterSecurity(t *testing.T) (*Writer, *sql.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sec.mpr")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE Unit (
			UnitID BLOB PRIMARY KEY NOT NULL,
			ContainerID BLOB,
			ContainmentName TEXT,
			TreeConflict LONG,
			ContentsHash TEXT,
			ContentsConflicts TEXT,
			Contents BLOB
		)
	`); err != nil {
		t.Fatalf("create Unit table: %v", err)
	}

	reader := &Reader{db: db, version: MPRVersionV1}
	return &Writer{reader: reader}, db
}

// seedDomainModelUnit inserts a minimal domain model BSON with one entity+attribute.
// Returns the unit ID and the domain model BSON (as bson.D).
func seedDomainModelUnit(t *testing.T, w *Writer, db *sql.DB) (unitID model.ID, entityID model.ID) {
	t.Helper()

	unitIDStr := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	containerIDStr := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	entityIDStr := "cccccccc-cccc-cccc-cccc-cccccccccccc"

	dmBSON := bson.D{
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "$ID", Value: idToBsonBinary(unitIDStr)},
		{Key: "Entities", Value: bson.A{
			int32(3),
			bson.D{
				{Key: "$Type", Value: "DomainModels$Entity"},
				{Key: "$ID", Value: idToBsonBinary(entityIDStr)},
				{Key: "Name", Value: "Order"},
				{Key: "Attributes", Value: bson.A{
					int32(3),
					bson.D{
						{Key: "$Type", Value: "DomainModels$StoredValue"},
						{Key: "$ID", Value: idToBsonBinary("dddddddd-dddd-dddd-dddd-dddddddddddd")},
						{Key: "Name", Value: "Status"},
					},
				}},
				{Key: "AccessRules", Value: bson.A{int32(3)}},
			},
		}},
		{Key: "Associations", Value: bson.A{int32(3)}},
	}

	contents, err := bson.Marshal(dmBSON)
	if err != nil {
		t.Fatalf("marshal domain model: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO Unit (UnitID, ContainerID, ContainmentName, TreeConflict, ContentsHash, ContentsConflicts, Contents)
		VALUES (?, ?, 'DomainModel', 0, ?, '', ?)`,
		uuidToBlob(unitIDStr),
		uuidToBlob(containerIDStr),
		contentHashBase64(contents),
		contents,
	); err != nil {
		t.Fatalf("insert domain model unit: %v", err)
	}

	return model.ID(unitIDStr), model.ID(entityIDStr)
}

// TestAddEntityAccessRule_XPathConstraint_FullRoundtrip verifies the complete
// flow for issue #431: AddEntityAccessRule + ReconcileMemberAccesses + parseDomainModel.
// Ensures that XPath and read/write rights survive the full write-then-read cycle.
func TestAddEntityAccessRule_XPathConstraint_FullRoundtrip(t *testing.T) {
	w, db := newTestWriterSecurity(t)
	unitID, _ := seedDomainModelUnit(t, w, db)
	containerIDStr := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	err := w.AddEntityAccessRule(
		unitID, "Order",
		[]string{"MyModule.User"},
		false, false,
		"ReadWrite",
		"[Status = 'Open']",
		[]EntityMemberAccess{
			{AttributeRef: "MyModule.Order.Status", AccessRights: "ReadWrite"},
		},
	)
	if err != nil {
		t.Fatalf("AddEntityAccessRule: %v", err)
	}

	// ReconcileMemberAccesses is called by execGrantEntityAccess right after
	count, err := w.ReconcileMemberAccesses(unitID, "MyModule")
	if err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}
	_ = count

	// Read back via parseDomainModel (same path as GetDomainModel)
	dm, err := w.reader.GetDomainModel(model.ID(containerIDStr))
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}

	if len(dm.Entities) == 0 {
		t.Fatal("no entities found in domain model")
	}

	var order *domainmodel.Entity
	for _, e := range dm.Entities {
		if e.Name == "Order" {
			order = e
			break
		}
	}
	if order == nil {
		t.Fatal("Order entity not found")
	}

	if len(order.AccessRules) == 0 {
		t.Fatal("AccessRules empty after AddEntityAccessRule + ReconcileMemberAccesses (issue #431)")
	}

	rule := order.AccessRules[0]
	if rule.XPathConstraint != "[Status = 'Open']" {
		t.Errorf("XPathConstraint = %q, want %q", rule.XPathConstraint, "[Status = 'Open']")
	}
	if rule.DefaultMemberAccessRights != domainmodel.MemberAccessRightsReadWrite {
		t.Errorf("DefaultMemberAccessRights = %q, want ReadWrite", rule.DefaultMemberAccessRights)
	}
	if len(rule.ModuleRoleNames) == 0 || rule.ModuleRoleNames[0] != "MyModule.User" {
		t.Errorf("ModuleRoleNames = %v, want [MyModule.User]", rule.ModuleRoleNames)
	}
	if len(rule.MemberAccesses) == 0 {
		t.Error("MemberAccesses empty after reconciliation")
	}
}

// TestAddEntityAccessRule_XPathConstraint_PreservesRights verifies that granting
// entity access with an XPath WHERE clause correctly persists both the XPath
// and the read/write rights (issue #431: rights were silently dropped).
func TestAddEntityAccessRule_XPathConstraint_PreservesRights(t *testing.T) {
	w, db := newTestWriterSecurity(t)
	unitID, _ := seedDomainModelUnit(t, w, db)

	err := w.AddEntityAccessRule(
		unitID, "Order",
		[]string{"MyModule.User"},
		false, false,
		"ReadWrite",
		"[Status = 'Open']",
		[]EntityMemberAccess{
			{AttributeRef: "MyModule.Order.Status", AccessRights: "ReadWrite"},
		},
	)
	if err != nil {
		t.Fatalf("AddEntityAccessRule: %v", err)
	}

	// Read back via parseDomainModel
	row := db.QueryRow(`SELECT Contents FROM Unit WHERE UnitID = ?`, uuidToBlob(string(unitID)))
	var contents []byte
	if err := row.Scan(&contents); err != nil {
		t.Fatalf("read unit contents: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	entities := extractBsonArray(raw["Entities"])
	if len(entities) == 0 {
		t.Fatal("no entities found after AddEntityAccessRule")
	}

	entityMap, ok := entities[0].(map[string]any)
	if !ok {
		t.Fatalf("entity is %T, want map[string]any", entities[0])
	}

	rules := extractBsonArray(entityMap["AccessRules"])
	if len(rules) == 0 {
		t.Fatal("AccessRules is empty after grant — rights not persisted (issue #431)")
	}

	ruleMap, ok := rules[0].(map[string]any)
	if !ok {
		t.Fatalf("rule is %T, want map[string]any", rules[0])
	}

	xpath := extractString(ruleMap["XPathConstraint"])
	if xpath != "[Status = 'Open']" {
		t.Errorf("XPathConstraint = %q, want %q", xpath, "[Status = 'Open']")
	}

	defaultAccess := extractString(ruleMap["DefaultMemberAccessRights"])
	if defaultAccess != "ReadWrite" {
		t.Errorf("DefaultMemberAccessRights = %q, want %q", defaultAccess, "ReadWrite")
	}

	memberAccesses := extractBsonArray(ruleMap["MemberAccesses"])
	if len(memberAccesses) == 0 {
		t.Error("MemberAccesses is empty after grant")
	}
}
