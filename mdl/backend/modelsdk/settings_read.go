// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genSet "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/settings"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
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
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
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

func modelSettingsFromGen(p *genSet.RuntimeSettings) *model.ModelSettings {
	ms := &model.ModelSettings{
		AfterStartupMicroflow:              p.AfterStartupMicroflowQualifiedName(),
		BeforeShutdownMicroflow:            p.BeforeShutdownMicroflowQualifiedName(),
		HealthCheckMicroflow:               p.HealthCheckMicroflowQualifiedName(),
		AllowUserMultipleSessions:          p.AllowUserMultipleSessions(),
		HashAlgorithm:                      p.HashAlgorithm(),
		BcryptCost:                         int(p.BcryptCost()),
		JavaVersion:                        p.JavaVersion(),
		RoundingMode:                       p.RoundingMode(),
		ScheduledEventTimeZoneCode:         p.ScheduledEventTimeZoneCode(),
		FirstDayOfWeek:                     p.FirstDayOfWeek(),
		DecimalScale:                       int(p.DecimalScale()),
		EnableDataStorageOptimisticLocking: p.EnableDataStorageOptimisticLocking(),
		UseDatabaseForeignKeyConstraints:   p.UseDatabaseForeignKeyConstraints(),
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
			HttpPortNumber:                int(cfg.RuntimePortNumber()),
			ServerPortNumber:              int(cfg.AdminPortNumber()),
			ApplicationRootUrl:            cfg.ApplicationRootUrl(),
			MaxJavaHeapSize:               int(cfg.MaxJavaHeapSize()),
			ExtraJvmParameters:            cfg.ExtraJvmParameters(),
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

// constantValueOf extracts a constant's configured value. The value lives in the
// nested SharedOrPrivateValue (a SharedValue); private values are not stored.
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
