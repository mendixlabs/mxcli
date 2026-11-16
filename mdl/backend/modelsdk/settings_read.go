// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genSet "github.com/mendixlabs/mxcli/modelsdk/gen/settings"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

func init() {
	// The BSON storage names "Settings$ModelSettings" and
	// "Settings$ConventionSettings" carry the same field layout as the SDK gen
	// types RuntimeSettings and ModelerSettings respectively (verified key-by-key
	// against the legacy parser). The engalar codegen registered only the SDK
	// qualified names, so without these aliases those parts decode to a bare
	// element.Base. Register the storage names as aliases so they decode into the
	// typed gen elements and their fields become readable.
	codec.DefaultRegistry.RegisterAlias("Settings$ModelSettings", "Settings$RuntimeSettings")
	codec.DefaultRegistry.RegisterAlias("Settings$ConventionSettings", "Settings$ModelerSettings")
	// A server configuration is stored as "Settings$ServerConfiguration" but the
	// gen type registers under the SDK name "Settings$Configuration"; without this
	// alias the ConfigurationSettings.Configurations children decode to bare
	// element.Base and the read surfaces zero configurations (ALTER SETTINGS
	// CONFIGURATION then can't find 'Default').
	codec.DefaultRegistry.RegisterAlias("Settings$ServerConfiguration", "Settings$Configuration")
}

// GetProjectSettings reads the Settings$ProjectSettings document (a versioned
// "Settings" array of polymorphic parts) through the codec engine and converts
// it to the semantic model.ProjectSettings.
func (b *Backend) GetProjectSettings() (*model.ProjectSettings, error) {
	g, err := mprread.GetProjectSettings(b.reader)
	if err != nil {
		return nil, err
	}
	ps := projectSettingsFromGen(g)
	// Capture each settings part's raw BSON so UpdateProjectSettings can overlay
	// modified fields onto the preserved part and pass every untouched part
	// through byte-for-byte (ADR-0005 guard-don't-drop). Without RawParts an
	// ALTER SETTINGS would have nothing to rewrite.
	ps.RawParts = b.readSettingsRawParts(string(g.ID()))
	return ps, nil
}

// readSettingsRawParts decodes the Settings$ProjectSettings unit and returns each
// element of its Settings array as a map (the versioned-array marker is skipped).
func (b *Backend) readSettingsRawParts(unitID string) []map[string]any {
	raw, err := b.reader.GetRawUnitBytes(unitID)
	if err != nil {
		return nil
	}
	return settingsRawParts(raw)
}

// settingsRawParts returns each element of a Settings$ProjectSettings
// document's Settings array as a map, skipping the versioned-array marker.
func settingsRawParts(raw []byte) []map[string]any {
	var doc bson.M
	if err := bsoncompat.UnmarshalDocM(raw, &doc); err != nil {
		return nil
	}
	arr, ok := doc["Settings"].(bson.A)
	if !ok {
		return nil
	}
	parts := make([]map[string]any, 0, len(arr))
	for _, el := range arr {
		switch p := el.(type) {
		case bson.M:
			parts = append(parts, map[string]any(p))
		case map[string]any:
			parts = append(parts, p)
		}
		// non-map elements (the leading int32 marker) are skipped
	}
	return parts
}

// projectSettingsFromGen converts a decoded gen ProjectSettings to the semantic
// type, dispatching on each part's concrete gen type.
func projectSettingsFromGen(g *genSet.ProjectSettings) *model.ProjectSettings {
	ps := &model.ProjectSettings{}
	ps.ID = model.ID(g.ID())
	ps.TypeName = "Settings$ProjectSettings"

	for _, el := range g.SettingsPartsItems() {
		switch p := el.(type) {
		case *genSet.WebUIProjectSettingsPart:
			ps.WebUI = &model.WebUISettings{
				EnableMicroflowReachabilityAnalysis: p.EnableMicroflowReachabilityAnalysis(),
				UseOptimizedClient:                  p.UseOptimizedClient(),
				UrlPrefix:                           p.UrlPrefix(),
			}
			for _, item := range p.ThemeModuleOrderItems() {
				if e, ok := item.(*genSet.ThemeModuleEntry); ok && e.ModuleName() != "" {
					ps.WebUI.ThemeModuleOrder = append(ps.WebUI.ThemeModuleOrder, e.ModuleName())
				}
			}
			setBase(&ps.WebUI.BaseElement, p, "Forms$WebUIProjectSettingsPart")
		case *genSet.ConfigurationSettings:
			ps.Configuration = configurationSettingsFromGen(p)
		case *genSet.RuntimeSettings:
			ps.Model = modelSettingsFromGen(p)
		case *genSet.ModelerSettings:
			ps.Convention = &model.ConventionSettings{
				LowerCaseMicroflowVariables: p.LowerCaseMicroflowVariables(),
				DefaultAssociationStorage:   p.DefaultAssociationStorage(),
			}
			setBase(&ps.Convention.BaseElement, p, "Settings$ConventionSettings")
		case *genSet.LanguageSettings:
			ps.Language = languageSettingsFromGen(p)
		case *genSet.WorkflowsProjectSettingsPart:
			ps.Workflows = &model.WorkflowsSettings{
				UserEntity:                p.UserEntityQualifiedName(),
				DefaultTaskParallelism:    int(p.DefaultTaskParallelism()),
				WorkflowEngineParallelism: int(p.WorkflowEngineParallelism()),
			}
			ps.Workflows.Groups, ps.Workflows.GroupsIncomplete = workflowGroupsFromGen(p)
			setBase(&ps.Workflows.BaseElement, p, "Settings$WorkflowsProjectSettingsPart")
		case *genSet.DistributionSettings:
			ps.Distribution = &model.DistributionSettings{
				IsDistributable: p.IsDistributable(),
				Version:         p.Version(),
			}
			setBase(&ps.Distribution.BaseElement, p, "Settings$DistributionSettings")
		case *genSet.IntegrationProjectSettingsPart:
			ps.Integration = &model.IntegrationSettings{}
			setBase(&ps.Integration.BaseElement, p, "Settings$IntegrationProjectSettingsPart")
		case *genSet.CertificateSettings:
			ps.Certificate = &model.CertificateSettings{}
			setBase(&ps.Certificate.BaseElement, p, "Settings$CertificateSettings")
		case *genSet.JarDeploymentSettings:
			ps.JarDeployment = &model.JarDeploymentSettings{}
			setBase(&ps.JarDeployment.BaseElement, p, "Settings$JarDeploymentSettings")
		}
	}
	return ps
}

// rawInt reads an integer property from an element's stored BSON, falling back to
// the value the gen accessor produced when the key is absent or not an integer.
func rawInt(el element.Element, key string, fallback int32) int {
	v := el.Raw().Lookup(key)
	if i, ok := v.Int32OK(); ok {
		return int(i)
	}
	if i, ok := v.Int64OK(); ok {
		return int(i)
	}
	return int(fallback)
}

// javaVersionOf reads the runtime Java version under whichever key this Mendix
// version stores it: 11.6 writes "JavaVersion" ("Java21"), 11.12 renamed it to
// "JavaMajorVersion" ("21") and the gen accessor only knows the former.
// See settingsoverlay.JavaVersionKey (mendixlabs/mxcli#759).
func javaVersionOf(p *genSet.RuntimeSettings) string {
	if v, ok := p.Raw().Lookup("JavaMajorVersion").StringValueOK(); ok {
		return v
	}
	return p.JavaVersion()
}

func modelSettingsFromGen(p *genSet.RuntimeSettings) *model.ModelSettings {
	ms := &model.ModelSettings{
		AfterStartupMicroflow:              p.AfterStartupMicroflowQualifiedName(),
		BeforeShutdownMicroflow:            p.BeforeShutdownMicroflowQualifiedName(),
		HealthCheckMicroflow:               p.HealthCheckMicroflowQualifiedName(),
		AllowUserMultipleSessions:          p.AllowUserMultipleSessions(),
		HashAlgorithm:                      p.HashAlgorithm(),
		BcryptCost:                         int(p.BcryptCost()),
		JavaVersion:                        javaVersionOf(p),
		RoundingMode:                       p.RoundingMode(),
		ScheduledEventTimeZoneCode:         p.ScheduledEventTimeZoneCode(),
		DefaultTimeZoneCode:                p.DefaultTimeZoneCode(),
		FirstDayOfWeek:                     p.FirstDayOfWeek(),
		DecimalScale:                       int(p.DecimalScale()),
		EnableDataStorageOptimisticLocking: p.EnableDataStorageOptimisticLocking(),
		UseDatabaseForeignKeyConstraints:   p.UseDatabaseForeignKeyConstraints(),
		UseOQLVersion2:                     p.UseOQLVersion2(),
		UseSystemContextForBackgroundTasks: p.UseSystemContextForBackgroundTasks(),
		SslCertificateAlgorithm:            p.SslCertificateAlgorithm(),
	}
	setBase(&ms.BaseElement, p, "Settings$ModelSettings")
	return ms
}

func configurationSettingsFromGen(p *genSet.ConfigurationSettings) *model.ConfigurationSettings {
	cs := &model.ConfigurationSettings{}
	setBase(&cs.BaseElement, p, "Settings$ConfigurationSettings")
	for _, el := range p.ConfigurationsItems() {
		cfg, ok := el.(*genSet.Configuration)
		if !ok {
			continue
		}
		sc := &model.ServerConfiguration{
			Name:                          cfg.Name(),
			DatabaseType:                  cfg.DatabaseType(),
			DatabaseUrl:                   cfg.DatabaseUrl(),
			DatabaseName:                  cfg.DatabaseName(),
			DatabaseUserName:              cfg.DatabaseUserName(),
			DatabasePassword:              cfg.DatabasePassword(),
			DatabaseUseIntegratedSecurity: cfg.DatabaseUseIntegratedSecurity(),
			// The gen Configuration binds the two ports under their SDK names
			// (RuntimePortNumber / AdminPortNumber); Studio Pro stores them as
			// HttpPortNumber / ServerPortNumber, so the accessors always returned 0
			// and the overlay wrote that 0 straight back — every existing
			// configuration lost its ports on any settings write
			// (mendixlabs/mxcli#759). Read the stored keys, keeping the accessors as
			// the fallback in case a future version adopts the SDK spelling.
			HttpPortNumber:     rawInt(cfg, "HttpPortNumber", cfg.RuntimePortNumber()),
			ServerPortNumber:   rawInt(cfg, "ServerPortNumber", cfg.AdminPortNumber()),
			ApplicationRootUrl: cfg.ApplicationRootUrl(),
			MaxJavaHeapSize:    int(cfg.MaxJavaHeapSize()),
			ExtraJvmParameters: cfg.ExtraJvmParameters(),
		}
		setBase(&sc.BaseElement, cfg, "Settings$Configuration")
		for _, cvEl := range cfg.ConstantValuesItems() {
			cv, ok := cvEl.(*genSet.ConstantValue)
			if !ok {
				continue
			}
			// The gen ConstantValue hardcodes its constant reference under the BSON
			// key "Constant", but Studio Pro stores it as "ConstantId"; read the
			// real key from the raw element (ConstantQualifiedName() is empty here).
			constantID := cv.ConstantQualifiedName()
			if constantID == "" {
				if v, ok := cv.Raw().Lookup("ConstantId").StringValueOK(); ok {
					constantID = v
				}
			}
			mcv := &model.ConstantValue{
				ConstantId: constantID,
				Value:      constantValueOf(cv),
				IsPrivate:  isPrivateConstantValue(cv),
			}
			setBase(&mcv.BaseElement, cv, "Settings$ConstantValue")
			sc.ConstantValues = append(sc.ConstantValues, mcv)
		}
		cs.Configurations = append(cs.Configurations, sc)
	}
	return cs
}

func languageSettingsFromGen(p *genSet.LanguageSettings) *model.LanguageSettings {
	ls := &model.LanguageSettings{DefaultLanguageCode: p.DefaultLanguageCode()}
	setBase(&ls.BaseElement, p, "Settings$LanguageSettings")
	for _, el := range p.LanguagesItems() {
		lang, ok := el.(*genSet.Language)
		if !ok {
			continue
		}
		ls.Languages = append(ls.Languages, model.Language{
			Code:                 lang.Code(),
			CheckCompleteness:    lang.CheckCompleteness(),
			CustomDateFormat:     lang.CustomDateFormat(),
			CustomDateTimeFormat: lang.CustomDateTimeFormat(),
			CustomTimeFormat:     lang.CustomTimeFormat(),
		})
	}
	return ls
}

// isPrivateConstantValue reports whether an override's value is private — stored
// on the developer's workstation rather than in the shared model. Studio Pro marks
// that by nesting a Settings$PrivateValue, a type with no properties at all, in
// place of the Settings$SharedValue that would carry a value.
//
// The gen registry may not have a factory for the marker, so the decoded child can
// be a bare element.Base; match on the type name rather than the Go type.
func isPrivateConstantValue(cv *genSet.ConstantValue) bool {
	spv := cv.SharedOrPrivateValue()
	if spv == nil {
		return false
	}
	return spv.TypeName() == "Settings$PrivateValue"
}

// constantValueOf extracts a constant's configured value. The value lives in the
// nested SharedOrPrivateValue (a SharedValue); a private value is not in the model
// at all, so this returns "" and isPrivateConstantValue tells the two apart.
func constantValueOf(cv *genSet.ConstantValue) string {
	if v := cv.Value(); v != "" {
		return v
	}
	if sv, ok := cv.SharedOrPrivateValue().(*genSet.SharedValue); ok {
		return sv.Value()
	}
	return ""
}

// setBase copies the gen element's ID and the given storage type name onto a
// semantic BaseElement.
func setBase(b *model.BaseElement, el interface{ ID() element.ID }, typeName string) {
	b.ID = model.ID(el.ID())
	b.TypeName = typeName
}

// workflowGroupsFromGen converts the Groups part-list of a workflows settings
// part to the semantic model. The second return reports that some element of the
// stored list did not decode as a Settings$WorkflowGroup: the write path rebuilds
// the list from the returned slice, so such an element would be dropped by any
// rewrite, and UpdateProjectSettings refuses rather than lose it.
func workflowGroupsFromGen(p *genSet.WorkflowsProjectSettingsPart) ([]model.WorkflowGroup, bool) {
	items := p.GroupsItems()
	if len(items) == 0 {
		return nil, false
	}
	groups := make([]model.WorkflowGroup, 0, len(items))
	incomplete := false
	for _, it := range items {
		g, ok := it.(*genSet.WorkflowGroup)
		if !ok {
			incomplete = true
			continue
		}
		wg := model.WorkflowGroup{Name: g.Name(), Description: g.Description()}
		setBase(&wg.BaseElement, g, "Settings$WorkflowGroup")
		groups = append(groups, wg)
	}
	return groups, incomplete
}
