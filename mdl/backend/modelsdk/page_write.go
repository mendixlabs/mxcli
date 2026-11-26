// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDT "github.com/mendixlabs/mxcli/modelsdk/gen/datatypes"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func init() {
	// A page always emits its Parameters and Variables arrays (empty = marker 3).
	codec.RegisterTypeDefaults("Forms$Page", codec.TypeDefaults{
		MandatoryLists: []string{"Parameters", "Variables"},
	})
	// FormCall arguments use the typed-array marker 2 when populated (the codec
	// emits 3 for empty lists automatically).
	codec.RegisterListMarker("Forms$FormCallArgument", 2)
}

// CreatePage inserts a new Forms$Page document unit (header, layout call, the
// widget tree, parameters, and variables) via pageToGen.
func (b *Backend) CreatePage(page *pages.Page) error {
	// A pluggable widget's children are serialized while the executor builds the
	// page, before this call; drain any failure so an unsupported construct fails
	// the statement instead of silently landing as a widget with the piece missing.
	if err := takeChildSerializeErr(); err != nil {
		return fmt.Errorf("CreatePage: %w", err)
	}
	if page == nil {
		return fmt.Errorf("CreatePage: nil page")
	}
	if b.writer == nil {
		return fmt.Errorf("CreatePage: not connected for writing")
	}
	if page.ID == "" {
		page.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodePage(page, b.ProjectVersion(), nil)
	if err != nil {
		return fmt.Errorf("CreatePage: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(page.ID), string(page.ContainerID), "Documents", "Forms$Page", contents); err != nil {
		return fmt.Errorf("CreatePage: insert: %w", err)
	}
	return nil
}

// UpdatePage rewrites an existing Forms$Page unit in place, preserving its UUID.
// Used by CREATE OR REPLACE PAGE and the styling alterations, which rebuild the
// full page (header + widget tree) and replace the existing unit. Serialization
// is identical to CreatePage.
func (b *Backend) UpdatePage(page *pages.Page) error {
	// A pluggable widget's children are serialized while the executor builds the
	// page, before this call; drain any failure so an unsupported construct fails
	// the statement instead of silently landing as a widget with the piece missing.
	if err := takeChildSerializeErr(); err != nil {
		return fmt.Errorf("UpdatePage: %w", err)
	}
	if page == nil {
		return fmt.Errorf("UpdatePage: nil page")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdatePage: not connected for writing")
	}
	pv := b.ProjectVersion()
	contents, err := encodePage(page, pv, func(g *genPg.Page) {
		b.carryStoredPageHeader(page.ID, g, pv)
		b.carryNoActionExecution(page.ID, g)
		b.carryLayoutGridPreviewWidth(page.ID, g)
	})
	if err != nil {
		return fmt.Errorf("UpdatePage: encode: %w", err)
	}
	if b.reader != nil {
		if stored, err := b.reader.GetRawUnitBytes(string(page.ID)); err == nil {
			contents = carryStoredPopupClose(contents, stored)
		}
	}
	if err := b.writer.UpdateRawUnit(string(page.ID), contents); err != nil {
		return fmt.Errorf("UpdatePage: update: %w", err)
	}
	return nil
}

// DeletePage removes a page unit by ID.
func (b *Backend) DeletePage(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeletePage: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// carryStoredPageHeader copies the three editor-only properties off the stored
// unit onto a rebuilt page, so a rewrite does not move state nobody asked to
// change.
//
// pageToGen writes Autofocus, CanvasWidth and CanvasHeight as constants, which
// looked safe and is not: Studio Pro varies all three per page. Across the 67
// pages of ako/TestApp at 11.14.0 CanvasWidth took SEVEN distinct values (800
// ×33, 1198 ×20, 1200 ×4, 802 ×2, 900 ×2, 2000, 1800), Autofocus was Off on 9,
// and CanvasHeight 500 on one. The hardcoded 1200 therefore matched 4 of 67, so
// `describe page` → `exec` moved the canvas of the other 63 and the unit was
// rewritten where ADR-0008 would otherwise have elided the write entirely
// (ako/mxcli#541).
//
// Carried rather than spelled in MDL: none of the three has a spelling, none is
// something a script asks for, and a value nobody asked to change should not
// change. This is the same reasoning as Microflow.StableId in ADR-0008.
//
// A missing or unreadable stored unit leaves the defaults in place rather than
// failing the write — this is a fidelity improvement on a rewrite, not a
// precondition for one, and CreatePage has no stored document by definition.
func (b *Backend) carryStoredPageHeader(id model.ID, g *genPg.Page, pv *types.ProjectVersion) {
	if b.reader == nil || id == "" {
		return
	}
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		return
	}
	var stored bson.D
	if err := bson.Unmarshal(raw, &stored); err != nil {
		return
	}
	// Carrying a stored value is right — except below the floor, where the key
	// should not be in the document at all. A pre-#547 mxcli wrote Autofocus into
	// Mendix 10 projects, so the stored value may itself be the defect; carrying
	// it would make the rewrite preserve a key that project's metamodel does not
	// declare. Dropping it is the repair, not data loss: there is no version of
	// this property the project can express.
	if v := bsonnav.DGetString(stored, "Autofocus"); v != "" && pageSupportsAutofocus(pv) {
		g.SetAutofocus(v)
	}
	// Read width-agnostically. Studio Pro stores both canvas dimensions as
	// **int64** (measured on all 67 TestApp pages), so a `.(int32)` assertion —
	// the natural one, since the gen setter takes int32 — matches nothing and
	// silently yields zero. That is the bson-numeric-width pattern, and it got
	// this fix once already: the first version passed its unit test because the
	// test's own fixture wrote int32, and still moved the canvas of every real
	// document.
	//
	// 0 is not a canvas size Studio Pro stores, so it stands in for "absent";
	// writing it back would give the editor a zero-width page.
	if v := bsonInt(bsonnav.DGet(stored, "CanvasWidth")); v > 0 {
		g.SetCanvasWidth(int32(v))
	}
	if v := bsonInt(bsonnav.DGet(stored, "CanvasHeight")); v > 0 {
		g.SetCanvasHeight(int32(v))
	}
	// pageToGen writes ExportLevel "Hidden" as a constant, and MDL has no
	// spelling for a page's export level, so an API page came back Hidden (#816).
	if v := bsonnav.DGetString(stored, "ExportLevel"); v != "" {
		g.SetExportLevel(v)
	}
}

// bsonInt reads an integer stored at any BSON width, mirroring extractInt in
// modelsdk/mpr/parser.go. Mendix picks the width per property and Go's type
// switch is exact, so a narrow assertion fails silently rather than loudly.
func bsonInt(v any) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// popupDimension returns the pop-up width/height for the gen Page (int32).
// Studio Pro's own default is 0 (auto-size), so 0 is a valid value and is
// written through verbatim (issue #713); only a stray negative is clamped to 0.
func popupDimension(n int) int32 {
	if n < 0 {
		return 0
	}
	return int32(n)
}

// Mendix introduced these two Forms$Page properties after the oldest version
// mxcli supports (mendixmodelsdk, Page.versionInfo). A key the project's
// metamodel does not declare makes a document Studio Pro cannot open, and
// mxbuild does not catch it — see codec.Encoder.OmitKeys.
const (
	pageAutofocusMajor, pageAutofocusMinor = 11, 1
	pageVariablesMajor, pageVariablesMinor = 10, 17
)

// pageSupportsAutofocus / pageSupportsVariables report whether the project's
// version declares the property. An unreadable version omits, matching the
// page-parameter guard: an absent optional property is filled in on load, an
// unknown one is unopenable.
func pageSupportsAutofocus(pv *types.ProjectVersion) bool {
	return pv != nil && pv.IsAtLeast(pageAutofocusMajor, pageAutofocusMinor)
}

func pageSupportsVariables(pv *types.ProjectVersion) bool {
	return pv != nil && pv.IsAtLeast(pageVariablesMajor, pageVariablesMinor)
}

// versionLabel renders a project version for an error message, including the
// case where it could not be read at all.
func versionLabel(pv *types.ProjectVersion) string {
	if pv == nil {
		return "unknown"
	}
	if pv.ProductVersion != "" {
		return pv.ProductVersion
	}
	return fmt.Sprintf("%d.%d.%d", pv.MajorVersion, pv.MinorVersion, pv.PatchVersion)
}

// docEncoder returns an encoder that drops the version-floored keys this project
// cannot carry. Variables is emitted by the codec's Studio Pro defaults registry
// rather than by a gen property, so suppressing it is the encoder's job; a gen
// PartList has no "present but empty" state to leave unset.
func docEncoder(typeName string, pv *types.ProjectVersion) *codec.Encoder {
	if pageSupportsVariables(pv) {
		return &codec.Encoder{}
	}
	return &codec.Encoder{OmitKeys: map[string]map[string]bool{
		typeName: {"Variables": true},
	}}
}

// encodePage builds and serializes a Forms$Page for a project of this version.
// Both CreatePage and UpdatePage go through it, so the version guards cannot be
// applied on one path and forgotten on the other.
// carry, when non-nil, runs against the built document before it is encoded —
// UpdatePage uses it for carryStoredPageHeader. CreatePage passes nil: a new
// page has no stored document to carry from.
func encodePage(page *pages.Page, pv *types.ProjectVersion, carry func(*genPg.Page)) ([]byte, error) {
	// Suppressing the key is right for the empty list Studio Pro always writes.
	// It is not right for variables the script actually declared: dropping those
	// would leave a page whose widgets reference names that are no longer there
	// (CE1151), from a statement that reported success. Refuse instead
	// (guard-don't-drop, ADR-0005).
	if len(page.Variables) > 0 && !pageSupportsVariables(pv) {
		names := make([]string, 0, len(page.Variables))
		for _, v := range page.Variables {
			names = append(names, v.Name)
		}
		return nil, fmt.Errorf(
			"page %q declares page variable(s) %s, which Mendix introduced in %d.%d (project is %s)",
			page.Name, strings.Join(names, ", "), pageVariablesMajor, pageVariablesMinor, versionLabel(pv))
	}
	g, err := pageToGen(page, pv)
	if err != nil {
		return nil, err
	}
	g.SetID(element.ID(page.ID))
	if carry != nil {
		carry(g)
	}
	contents, err := docEncoder("Forms$Page", pv).Encode(g)
	if err != nil {
		return nil, err
	}
	// Also refused at the writer (canon/attributeref.go); checked here first so
	// the refusal names the page rather than its unit id.
	if err := canon.BareAttributeRefError(fmt.Sprintf("page %q", page.Name), contents); err != nil {
		return nil, err
	}
	return contents, nil
}

// pageToGen builds the full gen Page: header, layout call, the widget tree (under
// the layout call's form-call arguments), parameters, and variables.
func pageToGen(page *pages.Page, pv *types.ProjectVersion) (*genPg.Page, error) {
	out := genPg.NewPage()
	out.SetName(page.Name)
	out.SetDocumentation(page.Documentation)
	out.SetExcluded(page.Excluded)
	out.SetExportLevel("Hidden")
	if pageSupportsAutofocus(pv) {
		out.SetAutofocus("DesktopOnly")
	}
	out.SetCanvasWidth(1200)
	out.SetCanvasHeight(600)
	out.SetMarkAsUsed(page.MarkAsUsed)
	out.SetUrl(page.URL)
	// Names the widget whose action closes the page as a pop-up. Writing "" here
	// unconditionally wiped it on every rewrite — set on 9 of ako/TestApp's 67
	// pages (ako/mxcli#550).
	//
	// Deliberately NOT carried from the stored document the way the canvas
	// properties are: this one names a widget on the page, and a rewrite rebuilds
	// the widget tree from the statement. Carrying a name the new tree may not
	// contain would leave a dangling reference. DESCRIBE emits it, so the round
	// trip is closed by the statement rather than behind it.
	out.SetPopupCloseAction(page.PopupCloseAction)
	out.SetPopupWidth(popupDimension(page.PopupWidth))
	out.SetPopupHeight(popupDimension(page.PopupHeight))
	out.SetPopupResizable(page.PopupResizable)
	out.SetAllowedRolesQualifiedNames(moduleRoleNames(page.AllowedRoles))
	out.SetTitle(captionToGen(page.Title))
	// The page's own design properties; nil here wrote an empty list over every
	// stored page's (ako/mxcli#1678).
	out.SetAppearance(newAppearance(page.Class, page.Style, "", page.DesignProperties))

	if page.LayoutCall != nil {
		lc, err := layoutCallToGen(page.LayoutCall)
		if err != nil {
			return nil, err
		}
		out.SetLayoutCall(lc)
	}

	for _, p := range page.Parameters {
		out.AddParameters(pageParameterToGen(p, pv))
	}
	for _, v := range page.Variables {
		out.AddVariables(localVariableToGen(v))
	}
	return out, nil
}

// localVariableToGen builds a Forms$LocalVariable (a page-level variable: name,
// default-value expression, and data type). Without this, a column header or widget
// bound to $VarName dangles → CE1151 "Missing variable".
func localVariableToGen(v *pages.LocalVariable) element.Element {
	lv := genPg.NewLocalVariable()
	if v.ID != "" {
		lv.SetID(element.ID(v.ID))
	}
	assignID(lv)
	lv.SetName(v.Name)
	lv.SetDefaultValue(v.DefaultValue)
	lv.SetVariableType(localVarTypeToGen(v.VariableType, v.EnumerationRef))
	return lv
}

// localVarTypeToGen maps a LocalVariable's BSON $Type ("DataTypes$StringType", …)
// to the gen data-type element.
//
// The default is String, which is Mendix's own default for a page variable — but
// it used to swallow an ENUMERATION too, because the builder handed one over as
// an ObjectType and nothing here recognised that either. A type was lost twice
// and reported as "String" both times (upstream #977). The enumeration now
// arrives with its qualified name and is built as what it is.
func localVarTypeToGen(typeName, enumerationQN string) element.Element {
	var t element.Element
	switch typeName {
	case "DataTypes$IntegerType", "DataTypes$LongType":
		// gen has no Long: Mendix models both as the same data type here.
		t = genDT.NewIntegerType()
	case "DataTypes$BooleanType":
		t = genDT.NewBooleanType()
	case "DataTypes$DecimalType":
		t = genDT.NewDecimalType()
	case "DataTypes$DateTimeType":
		t = genDT.NewDateTimeType()
	case "DataTypes$EnumerationType":
		// Without a qualified name there is nothing to point at, and a by-name
		// reference Mendix resolves to null is worse than a String.
		if enumerationQN != "" {
			et := genDT.NewEnumerationType()
			et.SetEnumerationQualifiedName(enumerationQN)
			t = et
		} else {
			t = genDT.NewStringType()
		}
	default:
		t = genDT.NewStringType()
	}
	assignID(t)
	return t
}

// layoutCallToGen builds the Forms$LayoutCall (page → layout binding) with one
// Forms$FormCallArgument per placeholder, including each placeholder's widget.
func layoutCallToGen(lc *pages.LayoutCall) (*genPg.LayoutCall, error) {
	out := genPg.NewLayoutCall()
	assignID(out)
	out.SetLayoutQualifiedName(lc.LayoutName)
	for _, arg := range lc.Arguments {
		ga := genPg.NewLayoutCallArgument()
		// The gen mislabels this type; real BSON is Forms$FormCallArgument.
		ga.SetTypeName("Forms$FormCallArgument")
		assignID(ga)
		ga.SetParameterQualifiedName(string(arg.ParameterID))
		for _, w := range arg.Widgets {
			wg, err := widgetToGen(w)
			if err != nil {
				return nil, err
			}
			ga.AddWidgets(wg)
		}
		out.AddArguments(ga)
	}
	return out, nil
}

// pageParameterVersion is the Mendix version that introduced PageParameter's
// IsRequired and DefaultValue properties. The element itself is 9.4.0.
const pageParamOptionalMajor, pageParamOptionalMinor = 11, 5

// pageParameterToGen converts a page parameter, including its ParameterType — an
// entity (DataTypes$ObjectType) or a primitive (DataTypes$StringType, …). Without
// the type the parameter can't resolve (CE5601/CE5606).
//
// IsRequired and DefaultValue are written only on 11.5+. MDL cannot express
// either — every page parameter it writes is required with no default — so below
// 11.5 they carry no information and are simply two keys the project's metamodel
// does not declare. That is the CLAUDE.md "never invent a key" case: mxbuild
// accepts unknown properties, Studio Pro throws InvalidOperationException at
// MprProperty.cs. An unreadable version omits them, because an absent optional
// property is filled in on load while an unknown one is unopenable.
func pageParameterToGen(p *pages.PageParameter, pv *types.ProjectVersion) *genPg.PageParameter {
	gp := genPg.NewPageParameter()
	if p.ID != "" {
		gp.SetID(element.ID(p.ID))
	}
	assignID(gp)
	gp.SetName(p.Name)
	if pv != nil && pv.IsAtLeast(pageParamOptionalMajor, pageParamOptionalMinor) {
		gp.SetIsRequired(p.IsRequired)
		gp.SetDefaultValue(p.DefaultValue)
	}
	gp.SetParameterType(pageParamTypeToGen(p))
	return gp
}

// pageParamTypeToGen builds a page parameter's type: a DataTypes$ObjectType for an
// entity parameter, or the named primitive DataTypes type. p.TypeName carries the
// primitive's BSON $Type (e.g. "DataTypes$StringType") when set.
func pageParamTypeToGen(p *pages.PageParameter) element.Element {
	return paramTypeToGen(p.TypeName, p.EntityName)
}

// paramTypeToGen builds the ParameterType child shared by Forms$PageParameter and
// Forms$SnippetParameter — both declare it as the polymorphic DataTypes$DataType,
// so one builder serves both. bsonType empty means "entity", and entityName is
// then the qualified name the DataTypes$ObjectType points at.
//
// Keeping the two on one builder is the point: a snippet parameter used to have
// its own, entity-only copy, so `Params: { $Label: String }` wrote a snippet
// parameter with no type at all (mendixlabs/mxcli#1028).
func paramTypeToGen(bsonType, entityName string) element.Element {
	if bsonType == "" {
		t := genDT.NewObjectType()
		assignID(t)
		t.SetEntityQualifiedName(entityName)
		return t
	}
	var t element.Element
	switch bsonType {
	case "DataTypes$IntegerType", "DataTypes$LongType":
		// gen has no Long: Mendix models both as the same data type, as
		// localVarTypeToGen notes for the sibling case. pageParamBSONType no
		// longer produces DataTypes$LongType, but a caller that sets TypeName
		// itself must not fall through to the String default the way one did.
		t = genDT.NewIntegerType()
	case "DataTypes$BooleanType":
		t = genDT.NewBooleanType()
	case "DataTypes$DecimalType":
		t = genDT.NewDecimalType()
	case "DataTypes$DateTimeType":
		t = genDT.NewDateTimeType()
	default:
		t = genDT.NewStringType()
	}
	assignID(t)
	return t
}

// storedPopupCloseKeys are the page properties Mendix 11.17 added for a popup's
// close behaviour (ako/mxcli#2612). MDL has no spelling for either.
var storedPopupCloseKeys = []string{"PopupOnCloseAction", "UseOldPopupCloseAction"}

// carryStoredPopupClose puts the stored page's popup close behaviour back on a
// rebuilt page that does not state it, so a rewrite does not change how the
// page's popups close.
//
// Measured: mx convert from 11.16.1 to 11.17.0 gave an existing page
// UseOldPopupCloseAction false and a Forms$CancelChangesClientAction, the same
// as every page of a blank 11.17.0 project — but a page can be set otherwise in
// Studio Pro, and canon.CompletePropertySets would fill the default, which
// changes behaviour nobody asked to change. Same reasoning as
// carryStoredPageHeader. A stored page without the keys (below 11.17) gives
// nothing to carry, so an older project gains no key its metamodel lacks.
func carryStoredPopupClose(contents, stored []byte) []byte {
	if len(stored) == 0 {
		return contents
	}
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return contents
	}
	have := map[string]bool{}
	for _, e := range doc {
		have[e.Key] = true
	}
	changed := false
	for _, k := range storedPopupCloseKeys {
		if have[k] {
			continue
		}
		v, err := bson.Raw(stored).LookupErr(k)
		if err != nil {
			continue
		}
		doc = append(doc, bson.E{Key: k, Value: v})
		changed = true
	}
	if !changed {
		return contents
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		return contents
	}
	return out
}
