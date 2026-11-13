// SPDX-License-Identifier: Apache-2.0

// Package settingsoverlay writes a project's server configurations back onto the
// BSON they were read from, instead of rebuilding them from the semantic model.
//
// model.ServerConfiguration carries only the fields mxcli understands. A
// configuration on disk carries more: CustomSettings, Tracing, the OpenAdminPort /
// OpenHttpPort flags the reader never populates, and whatever a newer Mendix adds.
// Serializing from the model alone therefore silently deletes all of it, and
// rewrites each constant override into the flat "Value" shape that Studio Pro and
// mxbuild ignore (mendixlabs/mxcli#801). Overlaying onto the preserved document is
// the ADR-0005 guard-don't-drop form of the same write.
//
// Both write engines share this package so the two cannot drift apart again: the
// codec engine (mdl/backend/modelsdk) and the legacy engine (sdk/mpr).
package settingsoverlay

import (
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
)

// DefaultListMarker is the leading version marker Studio Pro writes for the
// settings child lists (Configurations / ConstantValues / CustomSettings), matching
// the codec's default in modelsdk/codec.lookupListMarker. It is only a fallback: an
// existing list keeps whatever marker it already carries, since rebuilding one with
// a hardcoded marker silently downgrades it.
const DefaultListMarker = int32(3)

// PrivateValueType is the BSON $Type Studio Pro nests in a Settings$ConstantValue
// when the override's value is private: kept on the developer's workstation instead
// of in the shared model, so the model stores only this marker. Unlike
// Settings$SharedValue it defines no properties at all — writing one into it is the
// mendixlabs/mxcli#759 failure shape.
const PrivateValueType = "Settings$PrivateValue"

// SafeInt64 converts an int to int64 with a guard against the float64 safe-integer
// range (settings values are tiny, but keep the conversion bounds-checked).
func SafeInt64(v int) int64 {
	const maxSafe = 1 << 53
	if v > maxSafe {
		return maxSafe
	}
	if v < -maxSafe {
		return -maxSafe
	}
	return int64(v)
}

// Configurations rebuilds the Configurations list of a raw
// Settings$ConfigurationSettings part, overlaying each modelled configuration onto
// the raw configuration it was read from. A configuration read from disk carries
// its stored $ID and is paired by it; one without (built by hand) falls back to
// its name, which is how the executor addresses configurations (ALTER SETTINGS
// CONFIGURATION '<name>'). A modelled configuration with no match on disk is
// treated as newly created.
//
// Names are not unique: Studio Pro accepts two configurations called "Default"
// (a merge brings them in). Pairing by name alone put both modelled
// configurations onto the last stored document, the unit then held that $ID
// twice and every settings write was refused (ako/mxcli#2184). So each stored
// document is claimed at most once, and the id pass runs before the name pass.
//
// raw is mutated and returned.
func Configurations(cs *model.ConfigurationSettings, raw map[string]any) map[string]any {
	rawConfigs := ArrayElements(raw["Configurations"])
	claimed := make([]bool, len(rawConfigs))
	matched := make([]map[string]any, len(cs.Configurations))

	for i, cfg := range cs.Configurations {
		if cfg.ID == "" {
			continue
		}
		for j, rc := range rawConfigs {
			if !claimed[j] && rawIDString(rc["$ID"]) == strings.ToLower(string(cfg.ID)) {
				claimed[j], matched[i] = true, rc
				break
			}
		}
	}
	for i, cfg := range cs.Configurations {
		if matched[i] != nil {
			continue
		}
		for j, rc := range rawConfigs {
			name, _ := rc["Name"].(string)
			if !claimed[j] && strings.EqualFold(name, cfg.Name) {
				claimed[j], matched[i] = true, rc
				break
			}
		}
	}

	configs := bson.A{ArrayMarker(raw["Configurations"], DefaultListMarker)}
	for i, cfg := range cs.Configurations {
		configs = append(configs, ServerConfiguration(cfg, matched[i], rawConfigs))
	}
	raw["Configurations"] = configs
	return raw
}

// rawIDString returns a stored $ID as a lower-case UUID string, or "" when the
// value is absent or not an id.
func rawIDString(v any) string {
	switch id := v.(type) {
	case bson.Binary:
		return strings.ToLower(bsonutil.BsonBinaryToID(id))
	case string:
		return strings.ToLower(id)
	}
	return ""
}

// ServerConfiguration writes the fields the read path populates onto a
// configuration's preserved raw document, leaving every other key untouched. When
// raw is nil the configuration has no counterpart on disk (CREATE CONFIGURATION)
// and a fresh document is derived from siblings; pass nil siblings when there are
// none.
func ServerConfiguration(cfg *model.ServerConfiguration, raw map[string]any, siblings []map[string]any) map[string]any {
	if raw == nil {
		raw = newServerConfiguration(cfg, siblings)
	}
	raw["$Type"] = "Settings$ServerConfiguration"
	if raw["$ID"] == nil {
		raw["$ID"] = elementID(cfg.ID)
	}
	raw["Name"] = cfg.Name
	raw["DatabaseType"] = cfg.DatabaseType
	raw["DatabaseUrl"] = cfg.DatabaseUrl
	raw["DatabaseName"] = cfg.DatabaseName
	raw["DatabaseUserName"] = cfg.DatabaseUserName
	raw["DatabasePassword"] = cfg.DatabasePassword
	raw["DatabaseUseIntegratedSecurity"] = cfg.DatabaseUseIntegratedSecurity
	raw["HttpPortNumber"] = SafeInt64(cfg.HttpPortNumber)
	raw["ServerPortNumber"] = SafeInt64(cfg.ServerPortNumber)
	raw["ApplicationRootUrl"] = cfg.ApplicationRootUrl
	raw["MaxJavaHeapSize"] = SafeInt64(cfg.MaxJavaHeapSize)
	raw["ExtraJvmParameters"] = cfg.ExtraJvmParameters
	raw["ConstantValues"] = ConstantValues(cfg.ConstantValues, raw["ConstantValues"])
	return raw
}

// newServerConfiguration builds the raw document for a configuration that has no
// counterpart on disk. A sibling configuration is the shape template when one
// exists — it carries the exact field set and list markers this project's Mendix
// version writes — with the per-configuration collections emptied and the $ID
// dropped so a fresh one is minted. The tracing property is inherited from the
// sibling deliberately: it is version-specific (Mendix 11.6 stores "Tracing", 11.12
// "OpenTelemetry") and a new configuration has no better default to offer.
//
// Inherited by value, not by identity: every storage object nested in the copy
// gets a fresh $ID. On 11.12+ the OpenTelemetry part and its Logs / Traces children
// carry $IDs of their own, and sharing them put each id in the unit twice — the
// write path refused it ("element id(s) are used more than once"), so `create
// configuration` failed on every such project.
//
// With no sibling to copy there is no way to know which spelling this version
// expects, so the fallback writes neither: a property the metamodel does not define
// is what Studio Pro chokes on (see JavaVersionKey and mendixlabs/mxcli#759), while
// an absent optional property is filled in on load.
func newServerConfiguration(cfg *model.ServerConfiguration, siblings []map[string]any) map[string]any {
	if len(siblings) > 0 {
		tmpl := make(map[string]any, len(siblings[0]))
		for k, v := range siblings[0] {
			if k == "$ID" {
				continue // minted by ServerConfiguration
			}
			tmpl[k] = cloneWithFreshIDs(v)
		}
		tmpl["ConstantValues"] = bson.A{ArrayMarker(siblings[0]["ConstantValues"], DefaultListMarker)}
		tmpl["CustomSettings"] = bson.A{ArrayMarker(siblings[0]["CustomSettings"], DefaultListMarker)}
		return tmpl
	}
	return map[string]any{
		"ConstantValues": bson.A{DefaultListMarker},
		"CustomSettings": bson.A{DefaultListMarker},
		"OpenAdminPort":  cfg.OpenAdminPort,
		"OpenHttpPort":   cfg.OpenHttpPort,
	}
}

// cloneWithFreshIDs deep-copies a stored value, minting a new $ID for every
// storage object inside it. The copy shares no document with the original, so a
// later overlay of one cannot change the other.
func cloneWithFreshIDs(v any) any {
	if m, ok := AsMap(v); ok {
		out := make(map[string]any, len(m))
		for k, child := range m {
			if k == "$ID" {
				out[k] = bsonutil.NewIDBsonBinary()
				continue
			}
			out[k] = cloneWithFreshIDs(child)
		}
		return out
	}
	if arr, ok := v.(bson.A); ok {
		out := make(bson.A, len(arr))
		for i, el := range arr {
			out[i] = cloneWithFreshIDs(el)
		}
		return out
	}
	return v
}

// The two spellings of the runtime Java version property. They differ in value
// format as well as in name — see JavaVersionValue.
const (
	JavaMajorVersionKey = "JavaMajorVersion" // Mendix 11.12+, e.g. "21"
	JavaVersionEnumKey  = "JavaVersion"      // Mendix 11.6, e.g. "Java21"
)

// JavaVersionKey returns the storage key a Settings$ModelSettings part uses for
// the runtime Java version, or "" when it carries neither.
//
// Mendix renamed the property between 11.6 ("JavaVersion", values like "Java21")
// and 11.12 ("JavaMajorVersion", values like "21"). Writing a hardcoded
// "JavaVersion" onto an 11.12 document therefore left the real JavaMajorVersion
// stale and added a property that version's metamodel does not define — Studio Pro
// resolves each stored property against the type's property list and threw
// "Sequence contains no matching element" on the next open
// (mendixlabs/mxcli#759). Read the key off the document instead of assuming one,
// and never invent a key the document does not already have.
func JavaVersionKey(raw map[string]any) string {
	for _, k := range []string{JavaMajorVersionKey, JavaVersionEnumKey} {
		if _, ok := raw[k]; ok {
			return k
		}
	}
	return ""
}

// JavaVersion reads the runtime Java version from a raw Settings$ModelSettings
// part under whichever key this Mendix version stores it.
func JavaVersion(raw map[string]any) string {
	k := JavaVersionKey(raw)
	if k == "" {
		return ""
	}
	v, _ := raw[k].(string)
	return v
}

// SetJavaVersion writes the runtime Java version back to the key it was read from,
// in the value format that key expects. A part carrying neither key is left
// untouched.
func SetJavaVersion(raw map[string]any, v string) {
	if k := JavaVersionKey(raw); k != "" {
		raw[k] = JavaVersionValue(k, v)
	}
}

// JavaVersionValue renders a Java version in the form the given storage key holds:
// "JavaVersion" carries the enum member ("Java21"), "JavaMajorVersion" the bare
// major ("21").
//
// The rename in #759 changed the value format along with the key, and following
// only the key is not enough: Mendix 11.12 parses JavaMajorVersion with
// JavaVersionExtensions.fromString, which throws ArgumentOutOfRangeException
// ("majorVersion is an unsupported value: Java21") on the 11.6 spelling. So an
// `alter settings model JavaVersion = 'Java21'` written verbatim onto an 11.12
// document produces a project mx check refuses to load. Either spelling is
// accepted on input and stored in the document's own dialect.
//
// A value that is neither spelling — no recognisable major version — is passed
// through untouched, so a typo surfaces as a Mendix error rather than as a
// silently mangled setting.
func JavaVersionValue(key, v string) string {
	major := strings.TrimSpace(v)
	if len(major) >= 4 && strings.EqualFold(major[:4], "Java") {
		major = major[4:]
	}
	if major == "" || strings.TrimLeft(major, "0123456789") != "" {
		return v
	}
	if key == JavaMajorVersionKey {
		return major
	}
	return "Java" + major
}

// ModelSettingsKeys names every Settings$ModelSettings property mxcli parses and
// writes back, in the order DESCRIBE emits them. Which of these a document
// actually carries depends on the Mendix version — a blank 9.24 project stores 12
// of them, a blank 11.13 project stores 17 — so the overlay is presence-gated (see
// SetModelSettings) and callers must not assume any particular key exists.
var ModelSettingsKeys = []string{
	"AfterStartupMicroflow",
	"BeforeShutdownMicroflow",
	"HealthCheckMicroflow",
	"AllowUserMultipleSessions",
	"HashAlgorithm",
	"BcryptCost",
	"JavaVersion", // stored as JavaVersion or JavaMajorVersion; see JavaVersionKey
	"RoundingMode",
	"ScheduledEventTimeZoneCode",
	"DefaultTimeZoneCode",
	"FirstDayOfWeek",
	"DecimalScale",
	"EnableDataStorageOptimisticLocking",
	"UseDatabaseForeignKeyConstraints",
	"UseOQLVersion2",
	"UseSystemContextForBackgroundTasks",
	"SslCertificateAlgorithm",
}

// Has reports whether a raw BSON part carries a property at all. An absent
// optional property is not the same as one holding a zero value: Mendix fills it
// in from the type's default on load, so "absent" means "this version's default",
// not "false" or "0".
func Has(raw map[string]any, key string) bool {
	_, ok := raw[key]
	return ok
}

// setIfPresent writes a property back only if the stored document already carries
// it. Introducing one it does not carry is the mendixlabs/mxcli#759 failure shape:
// Studio Pro resolves every stored property against the type's property list and
// throws "Sequence contains no matching element" on one the type does not define,
// while mxbuild's deserializer tolerates it — so the build is not a safety net.
//
// Measured: before this gate, `alter settings model BcryptCost = 11` against a
// Mendix 9.24 project introduced DecimalScale = 0 and
// UseDatabaseForeignKeyConstraints = false, neither of which that version stores.
// Both were also wrong as values — 11.13 defaults them to 8 and true — so an
// unrelated one-line statement silently changed two other settings.
func setIfPresent(raw map[string]any, key string, v any) {
	if Has(raw, key) {
		raw[key] = v
	}
}

// SetModelSettings overlays parsed model settings onto the Settings$ModelSettings
// part they were read from. Shared by both write engines so the two cannot drift
// (the codec engine in mdl/backend/modelsdk and the legacy engine in sdk/mpr).
//
// Every property is presence-gated. A stored part always carries the core ones, so
// the gate is invisible there; it is load-bearing for the version-variable tail.
// The executor refuses an ALTER naming a property the document does not carry, so
// the gate never turns a user's request into a silent no-op.
func SetModelSettings(ms *model.ModelSettings, raw map[string]any) map[string]any {
	setIfPresent(raw, "AfterStartupMicroflow", ms.AfterStartupMicroflow)
	setIfPresent(raw, "BeforeShutdownMicroflow", ms.BeforeShutdownMicroflow)
	setIfPresent(raw, "HealthCheckMicroflow", ms.HealthCheckMicroflow)
	setIfPresent(raw, "AllowUserMultipleSessions", ms.AllowUserMultipleSessions)
	setIfPresent(raw, "HashAlgorithm", ms.HashAlgorithm)
	setIfPresent(raw, "BcryptCost", SafeInt64(ms.BcryptCost))
	SetJavaVersion(raw, ms.JavaVersion) // already presence-gated, and key-aware
	setIfPresent(raw, "RoundingMode", ms.RoundingMode)
	setIfPresent(raw, "ScheduledEventTimeZoneCode", ms.ScheduledEventTimeZoneCode)
	setIfPresent(raw, "DefaultTimeZoneCode", ms.DefaultTimeZoneCode)
	setIfPresent(raw, "FirstDayOfWeek", ms.FirstDayOfWeek)
	setIfPresent(raw, "DecimalScale", SafeInt64(ms.DecimalScale))
	setIfPresent(raw, "EnableDataStorageOptimisticLocking", ms.EnableDataStorageOptimisticLocking)
	setIfPresent(raw, "UseDatabaseForeignKeyConstraints", ms.UseDatabaseForeignKeyConstraints)
	setIfPresent(raw, "UseOQLVersion2", ms.UseOQLVersion2)
	setIfPresent(raw, "UseSystemContextForBackgroundTasks", ms.UseSystemContextForBackgroundTasks)
	setIfPresent(raw, "SslCertificateAlgorithm", ms.SslCertificateAlgorithm)
	return raw
}

// IsWebUIPart reports whether a settings part's $Type is the Web UI part. Studio
// Pro stores it as Forms$WebUIProjectSettingsPart (measured on 11.6, 11.13 and
// 11.14); the codec also registers the Settings$ spelling, and a part the reader
// decodes as Web UI settings has to be written back as one.
func IsWebUIPart(typeName string) bool {
	return typeName == "Forms$WebUIProjectSettingsPart" || typeName == "Settings$WebUIProjectSettingsPart"
}

// SetWebUISettings overlays the Web UI settings MDL can change onto the part they
// were read from. That is UseOptimizedClient alone — which client the app runs
// on — so every other property of the part passes through as stored. Presence-
// gated like SetModelSettings: the property was introduced in Mendix 9.10, and
// the executor refuses an ALTER on a part that does not carry it.
func SetWebUISettings(ws *model.WebUISettings, raw map[string]any) map[string]any {
	setIfPresent(raw, "UseOptimizedClient", ws.UseOptimizedClient)
	return raw
}

// ConstantValues rebuilds a configuration's ConstantValues list, updating each
// override in the slot it is already stored in so its value shape survives.
// From Mendix 10.9 Studio Pro and mxbuild only read the nested
// SharedOrPrivateValue, so writing every override flat made all constant
// overrides look empty in Studio Pro. Below 10.9 the flat "Value" is the only
// shape: a new override is still built nested here, and the write lowers it
// (canon.LowerToProjectShape, ako/mxcli#1163).
func ConstantValues(cvs []*model.ConstantValue, raw any) bson.A {
	rawValues := ArrayElements(raw)
	byConstant := make(map[string]map[string]any, len(rawValues))
	for _, rcv := range rawValues {
		id, _ := rcv["ConstantId"].(string)
		if id == "" {
			// The gen type binds the reference under "Constant"; Studio Pro writes
			// "ConstantId" (see modelsdk settings_read.configurationSettingsFromGen).
			id, _ = rcv["Constant"].(string)
		}
		if id != "" {
			byConstant[id] = rcv
		}
	}

	out := bson.A{ArrayMarker(raw, DefaultListMarker)}
	for _, cv := range cvs {
		out = append(out, constantValue(cv, byConstant[cv.ConstantId]))
	}
	return out
}

func constantValue(cv *model.ConstantValue, raw map[string]any) map[string]any {
	if raw == nil {
		// A new override has no stored shape to preserve, so write the one the
		// platform reads: the value nested in a Settings$SharedValue.
		return map[string]any{
			"$ID":        elementID(cv.ID),
			"$Type":      "Settings$ConstantValue",
			"ConstantId": cv.ConstantId,
			"SharedOrPrivateValue": map[string]any{
				"$ID":   bsonutil.NewIDBsonBinary(),
				"$Type": "Settings$SharedValue",
				"Value": cv.Value,
			},
		}
	}
	if shared, ok := AsMap(raw["SharedOrPrivateValue"]); ok {
		// A private override has no value in the model — Settings$PrivateValue is a
		// marker type with no properties, because the value lives on the developer's
		// workstation. Writing cv.Value (always "") into it would both fabricate a
		// property Mendix cannot resolve — the mendixlabs/mxcli#759 failure shape,
		// "Sequence contains no matching element" at MprProperty — and misreport the
		// override as empty. The shared/private choice belongs to the constant, not
		// to a configuration edit: preserve the node exactly as stored.
		if shared["$Type"] == PrivateValueType {
			return raw
		}
		shared["Value"] = cv.Value
		raw["SharedOrPrivateValue"] = shared
		// Clear a flat sibling rather than leave the two disagreeing: the reader
		// prefers a non-empty flat Value and would report a stale override.
		if _, has := raw["Value"]; has {
			raw["Value"] = ""
		}
		return raw
	}
	// Legacy flat-only shape: keep it flat rather than change the stored shape.
	raw["Value"] = cv.Value
	return raw
}

// ArrayMarker returns the leading version marker of a stored versioned array,
// falling back to def when the array is absent or unmarked.
func ArrayMarker(v any, def int32) int32 {
	arr, ok := v.(bson.A)
	if !ok || len(arr) == 0 {
		return def
	}
	switch m := arr[0].(type) {
	case int32:
		return m
	case int64:
		return int32(m)
	case int:
		return int32(m)
	}
	return def
}

// ArrayElements returns the document elements of a stored versioned array; the
// leading marker and any other non-document element are skipped.
func ArrayElements(v any) []map[string]any {
	arr, ok := v.(bson.A)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, el := range arr {
		if m, ok := AsMap(el); ok {
			out = append(out, m)
		}
	}
	return out
}

// AsMap normalises the two shapes bson.Unmarshal produces for a sub-document.
func AsMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case bson.M:
		return map[string]any(m), true
	case map[string]any:
		return m, true
	}
	return nil, false
}

// elementID returns the binary-UUID $ID for a settings sub-element, generating a
// fresh one when the model carries no ID (a newly-added configuration/override).
func elementID(id model.ID) any {
	if id != "" {
		return bsonutil.IDToBsonBinary(string(id))
	}
	return bsonutil.NewIDBsonBinary()
}

// Languages writes the ENABLED language list onto the preserved
// Settings$LanguageSettings document. The list is what App Settings ▸ Languages
// shows and the only thing a build emits translations for.
//
// Pinned against a Studio Pro-authored reference (11.13.0). Every field of the
// element it writes for a newly added language:
//
//	{ $Type: "Texts$Language", CheckCompleteness: false, Code: "ar_SD",
//	  CustomDateFormat: "", CustomDateTimeFormat: "", CustomTimeFormat: "" }
//
// Three things that reference settles, each of which a guess gets wrong:
//
//   - The element is **Texts$Language**, not Settings$Language, even though it
//     lives under Settings$LanguageSettings.
//   - There is **no Description**. Studio Pro's "Arabic, Sudan" is derived from
//     the code for display; the code is the whole stored identity.
//   - a newly added language starts with **CheckCompleteness false**. The flag is
//     a real setting — it makes Mendix report errors for texts with no
//     translation in that language — so it is written from the model, not forced.
//     The subtlety is the DEFAULT language: a stock project stores false for
//     en_US while the Languages table reads "Yes", because Mendix always checks
//     the default whatever the flag says.
//
// An existing language keeps its stored document, so its $ID and any property
// this mxcli does not model survive; only the modelled fields are overlaid, and
// the order is the semantic list's (Studio Pro appends rather than sorting).
func Languages(ls *model.LanguageSettings, raw map[string]any) map[string]any {
	rawLangs := ArrayElements(raw["Languages"])
	byCode := make(map[string]map[string]any, len(rawLangs))
	for _, rl := range rawLangs {
		code, _ := rl["Code"].(string)
		byCode[strings.ToLower(code)] = rl
	}

	langs := bson.A{ArrayMarker(raw["Languages"], DefaultListMarker)}
	for _, l := range ls.Languages {
		langs = append(langs, Language(l, byCode[strings.ToLower(l.Code)], rawLangs))
	}
	raw["Languages"] = langs
	return raw
}

// Language overlays one enabled language onto its preserved document. When raw is
// nil the language is newly enabled and a document is derived from a sibling —
// the same reasoning as newServerConfiguration: a sibling carries the exact field
// set this project's Mendix version writes, so nothing is invented.
func Language(l model.Language, raw map[string]any, siblings []map[string]any) map[string]any {
	if raw == nil {
		raw = newLanguage(siblings)
	}
	raw["$Type"] = "Texts$Language"
	if raw["$ID"] == nil {
		raw["$ID"] = elementID("")
	}
	raw["Code"] = l.Code
	raw["CheckCompleteness"] = l.CheckCompleteness
	raw["CustomDateFormat"] = l.CustomDateFormat
	raw["CustomTimeFormat"] = l.CustomTimeFormat
	raw["CustomDateTimeFormat"] = l.CustomDateTimeFormat
	return raw
}

// newLanguage builds the raw document for a language with no counterpart on disk.
// A sibling is the shape template when one exists; the fallback is the five
// properties the reference carries, which every project has at least one of
// (a project always enables its default language).
func newLanguage(siblings []map[string]any) map[string]any {
	if len(siblings) > 0 {
		tmpl := make(map[string]any, len(siblings[0]))
		for k, v := range siblings[0] {
			tmpl[k] = v
		}
		delete(tmpl, "$ID")
		return tmpl
	}
	return map[string]any{
		"CheckCompleteness":    false,
		"Code":                 "",
		"CustomDateFormat":     "",
		"CustomTimeFormat":     "",
		"CustomDateTimeFormat": "",
	}
}

// WorkflowGroupListMarker is the typed-array marker Studio Pro writes for the
// Groups list of a Settings$WorkflowsProjectSettingsPart. It is 2, not the 3 the
// other settings child lists use — measured on a blank Mendix 11.13.0 project,
// whose empty list is stored as `Groups: [2]`. It is only a fallback: a stored
// list keeps whatever marker it already carries.
const WorkflowGroupListMarker = int32(2)

// WorkflowGroups rebuilds the Groups list of a raw
// Settings$WorkflowsProjectSettingsPart, overlaying each modelled group onto the
// raw group it was read from. Groups are matched by name, which is also how the
// executor addresses one (ALTER SETTINGS WORKFLOWS ... GROUP '<name>') — a
// Settings$WorkflowGroup declares no identifier property, so the name is the
// identity.
//
// raw is mutated and returned.
func WorkflowGroups(ws *model.WorkflowsSettings, raw map[string]any) map[string]any {
	rawGroups := ArrayElements(raw["Groups"])
	byName := make(map[string]map[string]any, len(rawGroups))
	for _, rg := range rawGroups {
		name, _ := rg["Name"].(string)
		byName[strings.ToLower(name)] = rg
	}

	groups := bson.A{ArrayMarker(raw["Groups"], WorkflowGroupListMarker)}
	for _, g := range ws.Groups {
		groups = append(groups, WorkflowGroupDoc(g, byName[strings.ToLower(g.Name)]))
	}
	raw["Groups"] = groups
	return raw
}

// WorkflowGroupDoc overlays one group onto its preserved document. A group with
// no counterpart on disk is newly added, and unlike a language or a server
// configuration it needs no sibling to copy a shape from: Settings$WorkflowGroup
// declares exactly Name and Description and nothing else, in every source that
// describes the type (modelsdk/gen, generated/metamodel, mendixmodelsdk 4.115.0),
// so there is no version-specific spelling to guess at.
func WorkflowGroupDoc(g model.WorkflowGroup, raw map[string]any) map[string]any {
	if raw == nil {
		raw = map[string]any{}
	}
	raw["$Type"] = "Settings$WorkflowGroup"
	if raw["$ID"] == nil {
		raw["$ID"] = elementID(g.ID)
	}
	raw["Name"] = g.Name
	raw["Description"] = g.Description
	return raw
}
