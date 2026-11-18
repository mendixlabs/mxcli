// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// CE6630 "'Id' is marked Filterable=False in the OData service, but True in the
// app." Measured on MxBuild 11.16.1, one contract and one project per case, the
// importer (`create external entities from`) against: no annotation, a whole set
// that is not sortable/filterable, NonSortableProperties / NonFilterableProperties
// naming the label or the key, TopSupported, SkipSupported, CountRestrictions,
// Insert/Update/DeleteRestrictions. Eleven of thirteen were clean; the two that
// were not both restricted FILTERING of the KEY: the whole set Filterable=false,
// and NonFilterableProperties naming the key. The attribute's own flag was right —
// it is the Rest$ODataKeyPart, which carries a second Filterable, that was always
// written true.
func keyFilterableContract(annotation string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Thing">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="Label" Type="Edm.String" MaxLength="100"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Thing" EntityType="App.Model.Thing">` + annotation + `</EntitySet>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`
}

const (
	filterTerm  = `<Annotation Term="Org.OData.Capabilities.V1.FilterRestrictions">`
	sortTerm    = `<Annotation Term="Org.OData.Capabilities.V1.SortRestrictions">`
	wholeSetOff = `<Record><PropertyValue Property="Filterable" Bool="false"/></Record></Annotation>`
)

func nonFilterable(name string) string {
	return filterTerm + `<Record><PropertyValue Property="NonFilterableProperties"><Collection><PropertyPath>` + name +
		`</PropertyPath></Collection></PropertyValue></Record></Annotation>`
}

func TestImportedKeyPart_FilterableFollowsTheContract(t *testing.T) {
	cases := []struct {
		name, annotation string
		wantNotFilter    bool
	}{
		{"no annotation (control)", "", false},
		{"the whole set is not filterable", filterTerm + wholeSetOff, true},
		{"the key is listed as not filterable", nonFilterable("Id"), true},
		{"another property is listed (control)", nonFilterable("Label"), false},
		{"the whole set is not SORTABLE (control)", sortTerm + strings.Replace(wholeSetOff, "Filterable", "Sortable", 1), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entities, _ := importComplexTypeContract(t, keyFilterableContract(c.annotation))
			e := entities["Thing"]
			if e == nil || len(e.RemoteKeyParts) != 1 {
				t.Fatalf("want an imported Thing with one key part, got %+v", e)
			}
			if got := e.RemoteKeyParts[0].NotFilterable; got != c.wantNotFilter {
				t.Errorf("key part NotFilterable = %v, want %v", got, c.wantNotFilter)
			}
			// The attribute's own flag was already right and must stay in step.
			for _, a := range e.Attributes {
				if a.Name == "Id" && a.Filterable == c.wantNotFilter {
					t.Errorf("attribute Id Filterable = %v disagrees with the key part (NotFilterable %v)", a.Filterable, c.wantNotFilter)
				}
			}
		})
	}
}
