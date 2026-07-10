package codec_test

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestPatchBSONField_Update(t *testing.T) {
	doc := bson.D{{Key: "Name", Value: "old"}, {Key: "Other", Value: int32(42)}}
	data, _ := bson.Marshal(doc)

	result, err := codec.PatchBSONField(data, "Name", "new")
	if err != nil {
		t.Fatal(err)
	}

	var out bson.D
	if err := bson.Unmarshal(result, &out); err != nil {
		t.Fatal(err)
	}
	for _, e := range out {
		if e.Key == "Name" {
			if e.Value != "new" {
				t.Errorf("expected Name=new, got %v", e.Value)
			}
			return
		}
	}
	t.Error("Name field not found")
}

func TestPatchBSONField_Append(t *testing.T) {
	doc := bson.D{{Key: "Name", Value: "test"}}
	data, _ := bson.Marshal(doc)

	result, err := codec.PatchBSONField(data, "NewKey", "value")
	if err != nil {
		t.Fatal(err)
	}

	var out bson.D
	bson.Unmarshal(result, &out)
	if len(out) != 2 {
		t.Errorf("expected 2 fields, got %d", len(out))
	}
}

func TestPatchBSONArrayAppend(t *testing.T) {
	doc := bson.D{{Key: "Roles", Value: bson.A{int32(3), "Admin"}}}
	data, _ := bson.Marshal(doc)

	result, err := codec.PatchBSONArrayAppend(data, "Roles", "User")
	if err != nil {
		t.Fatal(err)
	}

	var out bson.D
	bson.Unmarshal(result, &out)
	for _, e := range out {
		if e.Key == "Roles" {
			arr := e.Value.(bson.A)
			if len(arr) != 3 {
				t.Errorf("expected 3 elements, got %d", len(arr))
			}
			if arr[2] != "User" {
				t.Errorf("expected last element User, got %v", arr[2])
			}
			return
		}
	}
	t.Error("Roles field not found")
}

func TestPatchBSONArrayRemove(t *testing.T) {
	doc := bson.D{{Key: "Roles", Value: bson.A{int32(3), "Admin", "User"}}}
	data, _ := bson.Marshal(doc)

	result, err := codec.PatchBSONArrayRemove(data, "Roles", "Admin")
	if err != nil {
		t.Fatal(err)
	}

	var out bson.D
	bson.Unmarshal(result, &out)
	for _, e := range out {
		if e.Key == "Roles" {
			arr := e.Value.(bson.A)
			if len(arr) != 2 { // version marker + User
				t.Errorf("expected 2 elements, got %d", len(arr))
			}
			return
		}
	}
	t.Error("Roles field not found")
}

func TestReadBSONFieldString(t *testing.T) {
	doc := bson.D{{Key: "Name", Value: "test"}}
	data, _ := bson.Marshal(doc)

	val, err := codec.ReadBSONFieldString(data, "Name")
	if err != nil {
		t.Fatal(err)
	}
	if val != "test" {
		t.Errorf("expected test, got %s", val)
	}
}

func TestReadBSONFieldString_NotFound(t *testing.T) {
	doc := bson.D{{Key: "Other", Value: "test"}}
	data, _ := bson.Marshal(doc)

	_, err := codec.ReadBSONFieldString(data, "Name")
	if err == nil {
		t.Error("expected error for missing field")
	}
}

func TestPatchNestedRefList(t *testing.T) {
	inner := bson.D{
		{Key: "Name", Value: "Admin"},
		{Key: "ModuleRoles", Value: bson.A{int32(3)}},
	}
	outer := bson.D{
		{Key: "UserRoles", Value: bson.A{int32(3), inner}},
	}
	data, _ := bson.Marshal(outer)

	result, err := codec.PatchNestedRefList(data, "UserRoles", "Name", "Admin", "ModuleRoles", []string{"MyModule.User"})
	if err != nil {
		t.Fatal(err)
	}

	var out bson.D
	bson.Unmarshal(result, &out)
	for _, e := range out {
		if e.Key == "UserRoles" {
			arr := e.Value.(bson.A)
			for _, item := range arr[1:] {
				d := item.(bson.D)
				for _, f := range d {
					if f.Key == "ModuleRoles" {
						roles := f.Value.(bson.A)
						if len(roles) != 2 { // version marker + 1 role
							t.Errorf("expected 2, got %d", len(roles))
						}
						if roles[1] != "MyModule.User" {
							t.Errorf("expected MyModule.User, got %v", roles[1])
						}
					}
				}
			}
			return
		}
	}
	t.Error("UserRoles not found")
}

func TestPatchDocumentAllowedRoles_Add(t *testing.T) {
	doc := bson.D{{Key: "AllowedRoles", Value: bson.A{int32(3), "Admin"}}}
	data, _ := bson.Marshal(doc)

	result, err := codec.PatchDocumentAllowedRoles(data, "AllowedRoles", "add", []string{"User", "Admin"})
	if err != nil {
		t.Fatal(err)
	}

	var out bson.D
	bson.Unmarshal(result, &out)
	for _, e := range out {
		if e.Key == "AllowedRoles" {
			arr := e.Value.(bson.A)
			// version marker + Admin (existing) + User (new) — Admin not duplicated
			if len(arr) != 3 {
				t.Errorf("expected 3, got %d: %v", len(arr), arr)
			}
			return
		}
	}
	t.Error("AllowedRoles not found")
}

func TestPatchDocumentAllowedRoles_Remove(t *testing.T) {
	doc := bson.D{{Key: "AllowedRoles", Value: bson.A{int32(3), "Admin", "User"}}}
	data, _ := bson.Marshal(doc)

	result, err := codec.PatchDocumentAllowedRoles(data, "AllowedRoles", "remove", []string{"Admin"})
	if err != nil {
		t.Fatal(err)
	}

	var out bson.D
	bson.Unmarshal(result, &out)
	for _, e := range out {
		if e.Key == "AllowedRoles" {
			arr := e.Value.(bson.A)
			if len(arr) != 2 { // version marker + User
				t.Errorf("expected 2, got %d: %v", len(arr), arr)
			}
			return
		}
	}
	t.Error("AllowedRoles not found")
}

func TestScanBSONStrings(t *testing.T) {
	doc := bson.D{
		{Key: "Name", Value: "hello"},
		{Key: "Count", Value: int32(5)},
		{Key: "Nested", Value: bson.D{
			{Key: "Inner", Value: "world"},
		}},
	}
	data, _ := bson.Marshal(doc)

	var found []string
	codec.ScanBSONStrings(data, func(path, val string) bool {
		found = append(found, path+"="+val)
		return true
	})

	if len(found) != 2 {
		t.Errorf("expected 2 strings, got %d: %v", len(found), found)
	}
	if found[0] != "Name=hello" {
		t.Errorf("expected Name=hello, got %s", found[0])
	}
	if found[1] != "Nested.Inner=world" {
		t.Errorf("expected Nested.Inner=world, got %s", found[1])
	}
}

func TestScanBSONStrings_EarlyStop(t *testing.T) {
	doc := bson.D{
		{Key: "A", Value: "first"},
		{Key: "B", Value: "second"},
		{Key: "C", Value: "third"},
	}
	data, _ := bson.Marshal(doc)

	count := 0
	codec.ScanBSONStrings(data, func(_, _ string) bool {
		count++
		return count < 2
	})

	if count != 2 {
		t.Errorf("expected early stop at 2, got %d", count)
	}
}
