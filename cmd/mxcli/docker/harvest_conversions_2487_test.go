// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"crypto/rand"
	"io"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2487, mendixlabs/mxcli#1917 (ako/mxcli#2492): Studio Pro 11.16.1
// records UseMeToSetupNodeConversion in Projects$ProjectConversion's
// OneTimeConversions; mx does not know that conversion (its provider is in the
// UI layer mx does not run), and drops the record whenever it re-saves the
// unit. `fix design-properties` — and the repair after `marketplace install`,
// which runs it — carried the re-saved unit back, so Studio Pro asked to
// convert the project again on the next open. Measured on 11.12.6 with the
// record added: `fix design-properties` renamed nothing and reported "1
// unit(s) changed", the record gone. A record a tool drops is put back; one it
// adds (it ran a conversion) is kept.

// conversionNames returns the OneTimeConversions names of the project's
// ProjectConversion unit, in order.
func conversionNames(t *testing.T, mprPath, unitID string) []string {
	t.Helper()
	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Disconnect()
	raw, err := reader.GetRawUnitBytes(model.ID(unitID))
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range doc {
		if e.Key != "OneTimeConversions" {
			continue
		}
		arr, _ := e.Value.(bson.A)
		for _, it := range arr {
			d, ok := it.(bson.D)
			if !ok {
				continue
			}
			for _, f := range d {
				if f.Key == "Name" {
					names = append(names, f.Value.(string))
				}
			}
		}
	}
	return names
}

// editConversions rewrites the unit's OneTimeConversions: drop removes a name,
// add appends one.
func editConversions(t *testing.T, mprPath, unitID, drop, add string) {
	t.Helper()
	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := reader.GetRawUnitBytes(model.ID(unitID))
	_ = reader.Disconnect()
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for i, e := range doc {
		if e.Key != "OneTimeConversions" {
			continue
		}
		arr, _ := e.Value.(bson.A)
		out := bson.A{}
		for _, it := range arr {
			if d, ok := it.(bson.D); ok {
				keep := true
				for _, f := range d {
					if f.Key == "Name" && f.Value == drop {
						keep = false
					}
				}
				if !keep {
					continue
				}
			}
			out = append(out, it)
		}
		if add != "" {
			out = append(out, bson.D{
				{Key: "$ID", Value: freshID2487()},
				{Key: "$Type", Value: "Projects$OneTimeConversion"},
				{Key: "Name", Value: add},
			})
		}
		doc[i].Value = out
	}
	encoded, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := openForWriting(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Disconnect()
	if err := writer.UpdateRawUnit(unitID, encoded); err != nil {
		t.Fatal(err)
	}
}

func contains2487(names []string, n string) bool {
	for _, x := range names {
		if x == n {
			return true
		}
	}
	return false
}

func TestHarvestKeepsStudioProConversionRecords_Issue2487(t *testing.T) {
	for _, subcommand := range []string{"rename-design-properties", "update-widgets"} {
		t.Run(subcommand, func(t *testing.T) {
			mprPath := v2Fixture(t)
			conv := unitOfType(t, mprPath, "Projects$ProjectConversion")
			// What Studio Pro 11.16.1 records on first open.
			editConversions(t, mprPath, conv, "", "UseMeToSetupNodeConversion")
			before := conversionNames(t, mprPath, conv)

			// What mx does: drop the record it does not know, and (a tool
			// that ran a conversion) record its own.
			stubMxTool(t, func(_ string, _ []string, _, _ io.Writer) error {
				editConversions(t, mprPath, conv, "UseMeToSetupNodeConversion", "ToolRanConversion")
				return nil
			})
			if _, err := RunToolPreservingFormat("mx", mprPath, subcommand, io.Discard, io.Discard); err != nil {
				t.Fatalf("RunToolPreservingFormat: %v", err)
			}
			after := conversionNames(t, mprPath, conv)
			if !contains2487(after, "UseMeToSetupNodeConversion") {
				t.Errorf("the Studio Pro conversion record was dropped: %d before, %d after", len(before), len(after))
			}
			for _, n := range before {
				if !contains2487(after, n) {
					t.Errorf("stored record %q lost", n)
				}
			}
			// update-widgets carries no ProjectConversion back at all (#2288),
			// so only a tool whose writes are carried back adds its record.
			if subcommand == "rename-design-properties" && !contains2487(after, "ToolRanConversion") {
				t.Error("the record the tool added was not kept")
			}
		})
	}
}

func freshID2487() bson.Binary {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return bson.Binary{Subtype: 0, Data: b}
}
