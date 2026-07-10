// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// safeInt64 converts an int to int64 with a guard against the float64 safe-integer
// range (settings values are tiny, but keep the conversion bounds-checked).
func safeInt64(v int) int64 {
	const maxSafe = 1 << 53
	if v > maxSafe {
		return maxSafe
	}
	if v < -maxSafe {
		return -maxSafe
	}
	return int64(v)
}

// UpdateProjectSettings rewrites the Settings$ProjectSettings unit using the
// raw-part overlay strategy (ADR-0005 guard-don't-drop): the Settings array is
// rebuilt from ps.RawParts (captured on read), and only the parsed-and-modified
// parts (Model / Configuration / Language / Workflows) have their fields overlaid
// onto the preserved raw part. Every other part (WebUI, Convention, Integration,
// Certificate, JarDeployment, Distribution, …) passes through byte-for-byte.
func (b *Backend) UpdateProjectSettings(ps *model.ProjectSettings) error {
	if ps == nil {
		return fmt.Errorf("UpdateProjectSettings: nil settings")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateProjectSettings: not connected for writing")
	}

	settings := bson.A{int32(2)} // versioned array prefix
	for _, rawPart := range ps.RawParts {
		typeName, _ := rawPart["$Type"].(string)
		switch typeName {
		case "Settings$ModelSettings":
			if ps.Model != nil {
				settings = append(settings, overlayModelSettings(ps.Model, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$ConfigurationSettings":
			if ps.Configuration != nil {
				settings = append(settings, overlayConfigurationSettings(ps.Configuration, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$LanguageSettings":
			if ps.Language != nil {
				rawPart["DefaultLanguageCode"] = ps.Language.DefaultLanguageCode
				settings = append(settings, rawPart)
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$WorkflowsProjectSettingsPart":
			if ps.Workflows != nil {
				rawPart["UserEntity"] = ps.Workflows.UserEntity
				rawPart["DefaultTaskParallelism"] = safeInt64(ps.Workflows.DefaultTaskParallelism)
				rawPart["WorkflowEngineParallelism"] = safeInt64(ps.Workflows.WorkflowEngineParallelism)
				settings = append(settings, rawPart)
			} else {
				settings = append(settings, rawPart)
			}
		default:
			// Preserve raw part as-is.
			settings = append(settings, rawPart)
		}
	}

	doc := bson.M{
		"$ID":      bsonutil.IDToBsonBinary(string(ps.ID)),
		"$Type":    "Settings$ProjectSettings",
		"Settings": settings,
	}
	// Mendix 11.12+ requires "$ID" first in every storage object; the raw-part
	// overlay above works with unordered maps, so order the whole tree on write.
	contents, err := bson.Marshal(bsonutil.OrderStorageValue(doc))
	if err != nil {
		return fmt.Errorf("UpdateProjectSettings: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(ps.ID), contents)
}

func overlayModelSettings(ms *model.ModelSettings, raw map[string]any) map[string]any {
	raw["AfterStartupMicroflow"] = ms.AfterStartupMicroflow
	raw["BeforeShutdownMicroflow"] = ms.BeforeShutdownMicroflow
	raw["HealthCheckMicroflow"] = ms.HealthCheckMicroflow
	raw["AllowUserMultipleSessions"] = ms.AllowUserMultipleSessions
	raw["HashAlgorithm"] = ms.HashAlgorithm
	raw["BcryptCost"] = safeInt64(ms.BcryptCost)
	raw["JavaVersion"] = ms.JavaVersion
	raw["RoundingMode"] = ms.RoundingMode
	raw["ScheduledEventTimeZoneCode"] = ms.ScheduledEventTimeZoneCode
	raw["FirstDayOfWeek"] = ms.FirstDayOfWeek
	raw["DecimalScale"] = safeInt64(ms.DecimalScale)
	raw["EnableDataStorageOptimisticLocking"] = ms.EnableDataStorageOptimisticLocking
	raw["UseDatabaseForeignKeyConstraints"] = ms.UseDatabaseForeignKeyConstraints
	return raw
}

func overlayConfigurationSettings(cs *model.ConfigurationSettings, raw map[string]any) map[string]any {
	configs := bson.A{int32(2)} // versioned array prefix
	for _, cfg := range cs.Configurations {
		configs = append(configs, serverConfigurationToBSON(cfg))
	}
	raw["Configurations"] = configs
	return raw
}

func serverConfigurationToBSON(cfg *model.ServerConfiguration) bson.M {
	cfgDoc := bson.M{
		"$ID":                           configID(cfg.ID),
		"$Type":                         "Settings$ServerConfiguration",
		"Name":                          cfg.Name,
		"DatabaseType":                  cfg.DatabaseType,
		"DatabaseUrl":                   cfg.DatabaseUrl,
		"DatabaseName":                  cfg.DatabaseName,
		"DatabaseUserName":              cfg.DatabaseUserName,
		"DatabasePassword":              cfg.DatabasePassword,
		"DatabaseUseIntegratedSecurity": cfg.DatabaseUseIntegratedSecurity,
		"HttpPortNumber":                safeInt64(cfg.HttpPortNumber),
		"ServerPortNumber":              safeInt64(cfg.ServerPortNumber),
		"ApplicationRootUrl":            cfg.ApplicationRootUrl,
		"MaxJavaHeapSize":               safeInt64(cfg.MaxJavaHeapSize),
		"ExtraJvmParameters":            cfg.ExtraJvmParameters,
		"OpenAdminPort":                 cfg.OpenAdminPort,
		"OpenHttpPort":                  cfg.OpenHttpPort,
		"CustomSettings":                bson.A{int32(2)},
		"Tracing":                       nil,
	}
	cvArr := bson.A{int32(2)} // versioned array prefix
	for _, cv := range cfg.ConstantValues {
		cvArr = append(cvArr, bson.M{
			"$ID":        configID(cv.ID),
			"$Type":      "Settings$ConstantValue",
			"ConstantId": cv.ConstantId,
			"Value":      cv.Value,
		})
	}
	cfgDoc["ConstantValues"] = cvArr
	return cfgDoc
}

// configID returns the binary-UUID $ID for a settings sub-element, generating a
// fresh one when the model carries no ID (a newly-added configuration/constant).
func configID(id model.ID) any {
	if id != "" {
		return bsonutil.IDToBsonBinary(string(id))
	}
	return bsonutil.NewIDBsonBinary()
}
