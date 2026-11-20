// SPDX-License-Identifier: Apache-2.0

package types

import (
	"strings"
	"testing"
)

// ako/mxcli#1977: a $metadata may declare a vocabulary alias —
// <edmx:Include Namespace="Org.OData.Capabilities.V1" Alias="Capabilities"/> —
// and write its terms with it (Term="Capabilities.UpdateRestrictions"). The
// terms were matched by their full namespace only, so an aliased annotation
// was skipped: an entity set imported as not creatable / updatable /
// deletable, which mxbuild (resolving the alias) reports as CE6630. Every term
// is now read through the document's aliases.
const aliasMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:Reference Uri="https://oasis-tcs.github.io/odata-vocabularies/vocabularies/Org.OData.Capabilities.V1.xml">
    <edmx:Include Namespace="Org.OData.Capabilities.V1" Alias="CAP"/>
  </edmx:Reference>
  <edmx:Reference Uri="https://oasis-tcs.github.io/odata-vocabularies/vocabularies/Org.OData.Core.V1.xml">
    <edmx:Include Namespace="Org.OData.Core.V1" Alias="CORE"/>
  </edmx:Reference>
  <edmx:DataServices>
    <Schema Namespace="Probe" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Order">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.Int64" Nullable="false">
          <Annotation Term="CORE.Computed" Bool="true"/>
        </Property>
        <Property Name="Number" Type="Edm.String">
          <Annotation Term="CORE.Immutable" Bool="true"/>
        </Property>
        <Annotation Term="CORE.Description" String="An order"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Orders" EntityType="Probe.Order">
          <Annotation Term="CAP.InsertRestrictions"><Record><PropertyValue Property="Insertable" Bool="true"/></Record></Annotation>
          <Annotation Term="CAP.UpdateRestrictions"><Record><PropertyValue Property="Updatable" Bool="true"/></Record></Annotation>
          <Annotation Term="CAP.TopSupported" Bool="false"/>
        </EntitySet>
        <EntitySet Name="Lines" EntityType="Probe.Order"/>
      </EntityContainer>
      <Annotations Target="Probe.Container/Lines">
        <Annotation Term="CAP.DeleteRestrictions"><Record><PropertyValue Property="Deletable" Bool="true"/></Record></Annotation>
      </Annotations>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

type aliasProbe struct {
	insertable, updatable, deletable, topSupported string
	computed, immutable                            bool
	description                                    string
}

func probeAliasDoc(t *testing.T, cap, core string) aliasProbe {
	t.Helper()
	src := strings.NewReplacer(`Alias="CAP"`, `Alias="Capabilities"`, `Alias="CORE"`, `Alias="Core"`,
		"CAP.", cap+".", "CORE.", core+".").Replace(aliasMetadata)
	doc, err := ParseEdmx(src)
	if err != nil {
		t.Fatal(err)
	}
	b := func(p *bool) string {
		if p == nil {
			return "unset"
		}
		if *p {
			return "true"
		}
		return "false"
	}
	var orders, lines *EdmEntitySet
	for _, es := range doc.EntitySets {
		switch es.Name {
		case "Orders":
			orders = es
		case "Lines":
			lines = es
		}
	}
	et := doc.Schemas[0].EntityTypes[0]
	return aliasProbe{
		insertable: b(orders.Insertable), updatable: b(orders.Updatable), topSupported: b(orders.TopSupported),
		deletable: b(lines.Deletable),
		computed:  et.Properties[0].Computed, immutable: et.Properties[1].Immutable,
		description: et.Summary,
	}
}

func TestParseEdmx_VocabularyAliases(t *testing.T) {
	full := probeAliasDoc(t, "Org.OData.Capabilities.V1", "Org.OData.Core.V1")
	want := aliasProbe{insertable: "true", updatable: "true", deletable: "true", topSupported: "false",
		computed: true, immutable: true, description: "An order"}
	if full != want {
		t.Fatalf("control, full namespaces: %+v, want %+v", full, want)
	}
	if aliased := probeAliasDoc(t, "Capabilities", "Core"); aliased != want {
		t.Errorf("aliased terms: %+v, want what the full namespaces give, %+v", aliased, want)
	}
	// Control: a prefix the document does not declare is not guessed at.
	unknown := probeAliasDoc(t, "Caps", "Kore")
	if unknown.updatable != "unset" || unknown.computed || unknown.description != "" {
		t.Errorf("an undeclared prefix was read as a vocabulary: %+v", unknown)
	}
}
