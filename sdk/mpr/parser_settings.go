// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// parseProjectSettings parses a Settings$ProjectSettings BSON document.
func (r *Reader) parseProjectSettings(unitID, containerID string, contents []byte) (*model.ProjectSettings, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	ps := &model.ProjectSettings{}
	ps.ID = model.ID(unitID)
	ps.TypeName = "Settings$ProjectSettings"

	// Parse Settings array (versioned: starts with int32(2))
	settingsArr := extractBsonArray(raw["Settings"])
	for _, s := range settingsArr {
		partMap := extractBsonMap(s)
		if partMap == nil {
			continue
		}
		// Preserve raw part for round-trip serialization
		ps.RawParts = append(ps.RawParts, partMap)

		typeName := extractString(partMap["$Type"])
		switch typeName {
		case "Forms$WebUIProjectSettingsPart":
			ps.WebUI = parseWebUISettings(partMap)
		case "Settings$IntegrationProjectSettingsPart":
			ps.Integration = &model.IntegrationSettings{}
			ps.Integration.ID = model.ID(extractBsonID(partMap["$ID"]))
			ps.Integration.TypeName = typeName
		case "Settings$ConfigurationSettings":
			ps.Configuration = parseConfigurationSettings(partMap)
		case "Settings$ModelSettings":
			ps.Model = parseModelSettings(partMap)
		case "Settings$ConventionSettings":
			ps.Convention = parseConventionSettings(partMap)
		case "Settings$LanguageSettings":
			ps.Language = parseLanguageSettings(partMap)
		case "Settings$CertificateSettings":
			ps.Certificate = &model.CertificateSettings{}
			ps.Certificate.ID = model.ID(extractBsonID(partMap["$ID"]))
			ps.Certificate.TypeName = typeName
		case "Settings$WorkflowsProjectSettingsPart":
			ps.Workflows = parseWorkflowsSettings(partMap)
		case "Settings$JarDeploymentSettings":
			ps.JarDeployment = &model.JarDeploymentSettings{}
			ps.JarDeployment.ID = model.ID(extractBsonID(partMap["$ID"]))
			ps.JarDeployment.TypeName = typeName
		case "Settings$DistributionSettings":
			ps.Distribution = parseDistributionSettings(partMap)
		}
	}

	return ps, nil
}

func parseWebUISettings(raw map[string]any) *model.WebUISettings {
	s := &model.WebUISettings{}
	s.ID = model.ID(extractBsonID(raw["$ID"]))
	s.TypeName = extractString(raw["$Type"])
	s.EnableMicroflowReachabilityAnalysis = extractBool(raw["EnableMicroflowReachabilityAnalysis"], false)
	s.UseOptimizedClient = extractString(raw["UseOptimizedClient"])
	s.UrlPrefix = extractString(raw["UrlPrefix"])
	return s
}

func parseConfigurationSettings(raw map[string]any) *model.ConfigurationSettings {
	cs := &model.ConfigurationSettings{}
	cs.ID = model.ID(extractBsonID(raw["$ID"]))
	cs.TypeName = extractString(raw["$Type"])

	configs := extractBsonArray(raw["Configurations"])
	for _, c := range configs {
		if cMap := extractBsonMap(c); cMap != nil {
			cs.Configurations = append(cs.Configurations, parseServerConfiguration(cMap))
		}
	}

	return cs
}

func parseServerConfiguration(raw map[string]any) *model.ServerConfiguration {
	sc := &model.ServerConfiguration{}
	sc.ID = model.ID(extractBsonID(raw["$ID"]))
	sc.TypeName = extractString(raw["$Type"])
	sc.Name = extractString(raw["Name"])
	sc.DatabaseType = extractString(raw["DatabaseType"])
	sc.DatabaseUrl = extractString(raw["DatabaseUrl"])
	sc.DatabaseName = extractString(raw["DatabaseName"])
	sc.DatabaseUserName = extractString(raw["DatabaseUserName"])
	sc.DatabasePassword = extractString(raw["DatabasePassword"])
	sc.DatabaseUseIntegratedSecurity = extractBool(raw["DatabaseUseIntegratedSecurity"], false)
	sc.HttpPortNumber = extractInt(raw["HttpPortNumber"])
	sc.ServerPortNumber = extractInt(raw["ServerPortNumber"])
	sc.ApplicationRootUrl = extractString(raw["ApplicationRootUrl"])
	sc.MaxJavaHeapSize = extractInt(raw["MaxJavaHeapSize"])
	sc.ExtraJvmParameters = extractString(raw["ExtraJvmParameters"])
	sc.OpenAdminPort = extractBool(raw["OpenAdminPort"], false)
	sc.OpenHttpPort = extractBool(raw["OpenHttpPort"], false)

	// Parse ConstantValues
	cvArr := extractBsonArray(raw["ConstantValues"])
	for _, cv := range cvArr {
		if cvMap := extractBsonMap(cv); cvMap != nil {
			sc.ConstantValues = append(sc.ConstantValues, parseConstantValue(cvMap))
		}
	}

	return sc
}

func parseConstantValue(raw map[string]any) *model.ConstantValue {
	cv := &model.ConstantValue{}
	cv.ID = model.ID(extractBsonID(raw["$ID"]))
	cv.TypeName = extractString(raw["$Type"])
	cv.ConstantId = extractString(raw["ConstantId"])

	// Value is nested in SharedOrPrivateValue → Value
	if spv := extractBsonMap(raw["SharedOrPrivateValue"]); spv != nil {
		cv.Value = extractString(spv["Value"])
	}

	return cv
}

func parseModelSettings(raw map[string]any) *model.ModelSettings {
	ms := &model.ModelSettings{}
	ms.ID = model.ID(extractBsonID(raw["$ID"]))
	ms.TypeName = extractString(raw["$Type"])
	ms.AfterStartupMicroflow = extractString(raw["AfterStartupMicroflow"])
	ms.BeforeShutdownMicroflow = extractString(raw["BeforeShutdownMicroflow"])
	ms.HealthCheckMicroflow = extractString(raw["HealthCheckMicroflow"])
	ms.AllowUserMultipleSessions = extractBool(raw["AllowUserMultipleSessions"], true)
	ms.HashAlgorithm = extractString(raw["HashAlgorithm"])
	ms.BcryptCost = extractInt(raw["BcryptCost"])
	ms.JavaVersion = extractString(raw["JavaVersion"])
	ms.RoundingMode = extractString(raw["RoundingMode"])
	ms.ScheduledEventTimeZoneCode = extractString(raw["ScheduledEventTimeZoneCode"])
	ms.FirstDayOfWeek = extractString(raw["FirstDayOfWeek"])
	ms.DecimalScale = extractInt(raw["DecimalScale"])
	ms.EnableDataStorageOptimisticLocking = extractBool(raw["EnableDataStorageOptimisticLocking"], false)
	ms.UseDatabaseForeignKeyConstraints = extractBool(raw["UseDatabaseForeignKeyConstraints"], false)
	return ms
}

func parseConventionSettings(raw map[string]any) *model.ConventionSettings {
	cs := &model.ConventionSettings{}
	cs.ID = model.ID(extractBsonID(raw["$ID"]))
	cs.TypeName = extractString(raw["$Type"])
	cs.LowerCaseMicroflowVariables = extractBool(raw["LowerCaseMicroflowVariables"], false)
	cs.DefaultAssociationStorage = extractString(raw["DefaultAssociationStorage"])
	return cs
}

func parseLanguageSettings(raw map[string]any) *model.LanguageSettings {
	ls := &model.LanguageSettings{}
	ls.ID = model.ID(extractBsonID(raw["$ID"]))
	ls.TypeName = extractString(raw["$Type"])
	ls.DefaultLanguageCode = extractString(raw["DefaultLanguageCode"])
	for _, item := range extractBsonArray(raw["Languages"]) {
		langMap := extractBsonMap(item)
		if langMap == nil {
			continue
		}
		ls.Languages = append(ls.Languages, model.Language{
			Code:                 extractString(langMap["Code"]),
			CheckCompleteness:    extractBool(langMap["CheckCompleteness"], false),
			CustomDateFormat:     extractString(langMap["CustomDateFormat"]),
			CustomDateTimeFormat: extractString(langMap["CustomDateTimeFormat"]),
			CustomTimeFormat:     extractString(langMap["CustomTimeFormat"]),
		})
	}
	return ls
}

func parseWorkflowsSettings(raw map[string]any) *model.WorkflowsSettings {
	ws := &model.WorkflowsSettings{}
	ws.ID = model.ID(extractBsonID(raw["$ID"]))
	ws.TypeName = extractString(raw["$Type"])
	ws.UserEntity = extractString(raw["UserEntity"])
	ws.DefaultTaskParallelism = extractInt(raw["DefaultTaskParallelism"])
	ws.WorkflowEngineParallelism = extractInt(raw["WorkflowEngineParallelism"])
	return ws
}

func parseDistributionSettings(raw map[string]any) *model.DistributionSettings {
	ds := &model.DistributionSettings{}
	ds.ID = model.ID(extractBsonID(raw["$ID"]))
	ds.TypeName = extractString(raw["$Type"])
	ds.IsDistributable = extractBool(raw["IsDistributable"], false)
	ds.Version = extractString(raw["Version"])
	return ds
}
