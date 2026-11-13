// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2133: describe left a data view's "no entity" message out, so a
// describe round trip dropped it; the builder never filled the field.
func TestDataViewNoEntityMessageIsBuilt(t *testing.T) {
	pb := &pageBuilder{paramEntityNames: map[string]string{}, widgetScope: map[string]model.ID{}}
	dv, err := pb.buildDataViewV3(dataViewWith(map[string]any{"NoEntityMessage": "Nothing selected"}))
	if err != nil {
		t.Fatal(err)
	}
	if dv.NoEntityMessage == nil || len(dv.NoEntityMessage.Translations) != 1 {
		t.Fatalf("NoEntityMessage = %+v, want one translation", dv.NoEntityMessage)
	}
	for _, v := range dv.NoEntityMessage.Translations {
		if v != "Nothing selected" {
			t.Errorf("translation = %q", v)
		}
	}
	// Control: no clause, no message.
	bare, _ := (&pageBuilder{paramEntityNames: map[string]string{}, widgetScope: map[string]model.ID{}}).buildDataViewV3(dataViewWith(map[string]any{}))
	if bare.NoEntityMessage != nil {
		t.Errorf("a data view with no clause got a message: %+v", bare.NoEntityMessage)
	}
}
