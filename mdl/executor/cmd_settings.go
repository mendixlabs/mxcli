// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// listSettings displays an overview table of all settings parts.
func listSettings(ctx *ExecContext) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	tr := &TableResult{
		Columns: []string{"Section", "Key Values"},
	}

	if ps.Model != nil {
		ms := ps.Model
		values := []string{}
		if ms.AfterStartupMicroflow != "" {
			values = append(values, "AfterStartup: "+ms.AfterStartupMicroflow)
		}
		values = append(values, "Hash: "+ms.HashAlgorithm)
		values = append(values, "Java: "+ms.JavaVersion)
		tr.Rows = append(tr.Rows, []any{"Model Settings", strings.Join(values, ", ")})
	}

	if ps.Configuration != nil {
		for _, cfg := range ps.Configuration.Configurations {
			values := []string{}
			values = append(values, cfg.DatabaseType)
			values = append(values, cfg.DatabaseUrl)
			values = append(values, "db="+cfg.DatabaseName)
			values = append(values, fmt.Sprintf("http=%d", cfg.HttpPortNumber))
			if len(cfg.ConstantValues) > 0 {
				values = append(values, fmt.Sprintf("%d constants", len(cfg.ConstantValues)))
			}
			tr.Rows = append(tr.Rows, []any{
				fmt.Sprintf("Configuration '%s'", cfg.Name),
				strings.Join(values, ", "),
			})
		}
	}

	if ps.Language != nil {
		tr.Rows = append(tr.Rows, []any{"Language Settings", "Default: " + ps.Language.DefaultLanguageCode})
	}

	if ps.Workflows != nil {
		ws := ps.Workflows
		values := []string{}
		if ws.UserEntity != "" {
			values = append(values, "UserEntity: "+ws.UserEntity)
		}
		if ws.DefaultTaskParallelism > 0 {
			values = append(values, fmt.Sprintf("TaskParallelism: %d", ws.DefaultTaskParallelism))
		}
		tr.Rows = append(tr.Rows, []any{"Workflow Settings", strings.Join(values, ", ")})
	}

	if ps.Convention != nil {
		tr.Rows = append(tr.Rows, []any{"Convention Settings", "AssocStorage: " + ps.Convention.DefaultAssociationStorage})
	}

	if ps.WebUI != nil {
		tr.Rows = append(tr.Rows, []any{"Web UI Settings", "OptimizedClient: " + ps.WebUI.UseOptimizedClient})
	}

	return writeResult(ctx, tr)
}

// describeSettings outputs the full MDL description of all settings.
func describeSettings(ctx *ExecContext) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	// Model settings
	if ps.Model != nil {
		ms := ps.Model
		var parts []string
		if ms.AfterStartupMicroflow != "" {
			parts = append(parts, fmt.Sprintf("  AfterStartupMicroflow = '%s'", ms.AfterStartupMicroflow))
		}
		if ms.BeforeShutdownMicroflow != "" {
			parts = append(parts, fmt.Sprintf("  BeforeShutdownMicroflow = '%s'", ms.BeforeShutdownMicroflow))
		}
		if ms.HealthCheckMicroflow != "" {
			parts = append(parts, fmt.Sprintf("  HealthCheckMicroflow = '%s'", ms.HealthCheckMicroflow))
		}
		parts = append(parts, fmt.Sprintf("  HashAlgorithm = '%s'", ms.HashAlgorithm))
		parts = append(parts, fmt.Sprintf("  BcryptCost = %d", ms.BcryptCost))
		parts = append(parts, fmt.Sprintf("  JavaVersion = '%s'", ms.JavaVersion))
		parts = append(parts, fmt.Sprintf("  RoundingMode = '%s'", ms.RoundingMode))
		parts = append(parts, fmt.Sprintf("  AllowUserMultipleSessions = %t", ms.AllowUserMultipleSessions))
		if ms.ScheduledEventTimeZoneCode != "" {
			parts = append(parts, fmt.Sprintf("  ScheduledEventTimeZoneCode = '%s'", ms.ScheduledEventTimeZoneCode))
		}
		fmt.Fprintf(ctx.Output, "alter settings model\n%s;\n\n", strings.Join(parts, ",\n"))
	}

	// Configuration settings
	if ps.Configuration != nil {
		for _, cfg := range ps.Configuration.Configurations {
			var parts []string
			parts = append(parts, fmt.Sprintf("  DatabaseType = '%s'", cfg.DatabaseType))
			parts = append(parts, fmt.Sprintf("  DatabaseUrl = '%s'", cfg.DatabaseUrl))
			parts = append(parts, fmt.Sprintf("  DatabaseName = '%s'", cfg.DatabaseName))
			parts = append(parts, fmt.Sprintf("  DatabaseUserName = '%s'", cfg.DatabaseUserName))
			parts = append(parts, fmt.Sprintf("  DatabasePassword = '%s'", cfg.DatabasePassword))
			parts = append(parts, fmt.Sprintf("  HttpPortNumber = %d", cfg.HttpPortNumber))
			parts = append(parts, fmt.Sprintf("  ServerPortNumber = %d", cfg.ServerPortNumber))
			if cfg.ApplicationRootUrl != "" {
				parts = append(parts, fmt.Sprintf("  ApplicationRootUrl = '%s'", cfg.ApplicationRootUrl))
			}
			fmt.Fprintf(ctx.Output, "alter settings configuration '%s'\n%s;\n\n", cfg.Name, strings.Join(parts, ",\n"))

			// Output constant overrides
			for _, cv := range cfg.ConstantValues {
				fmt.Fprintf(ctx.Output, "alter settings constant '%s' value '%s'\n  in configuration '%s';\n\n",
					cv.ConstantId, cv.Value, cfg.Name)
			}
		}
	}

	// Language settings
	if ps.Language != nil {
		fmt.Fprintf(ctx.Output, "alter settings LANGUAGE\n  DefaultLanguageCode = '%s';\n\n", ps.Language.DefaultLanguageCode)
	}

	// Workflow settings
	if ps.Workflows != nil {
		ws := ps.Workflows
		var parts []string
		if ws.UserEntity != "" {
			parts = append(parts, fmt.Sprintf("  UserEntity = '%s'", ws.UserEntity))
		}
		if ws.DefaultTaskParallelism > 0 {
			parts = append(parts, fmt.Sprintf("  DefaultTaskParallelism = %d", ws.DefaultTaskParallelism))
		}
		if ws.WorkflowEngineParallelism > 0 {
			parts = append(parts, fmt.Sprintf("  WorkflowEngineParallelism = %d", ws.WorkflowEngineParallelism))
		}
		if len(parts) > 0 {
			fmt.Fprintf(ctx.Output, "alter settings workflows\n%s;\n\n", strings.Join(parts, ",\n"))
		}
	}

	return nil
}

// alterSettings modifies project settings based on ALTER SETTINGS statement.
func alterSettings(ctx *ExecContext, stmt *ast.AlterSettingsStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	section := strings.ToLower(stmt.Section)
	switch section {
	case "model":
		if ps.Model == nil {
			return mdlerrors.NewNotFound("settings section", "model")
		}
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			switch key {
			case "AfterStartupMicroflow":
				ps.Model.AfterStartupMicroflow = valStr
			case "BeforeShutdownMicroflow":
				ps.Model.BeforeShutdownMicroflow = valStr
			case "HealthCheckMicroflow":
				ps.Model.HealthCheckMicroflow = valStr
			case "HashAlgorithm":
				ps.Model.HashAlgorithm = valStr
			case "BcryptCost":
				if v, err := strconv.Atoi(valStr); err == nil {
					ps.Model.BcryptCost = v
				}
			case "JavaVersion":
				ps.Model.JavaVersion = valStr
			case "RoundingMode":
				ps.Model.RoundingMode = valStr
			case "AllowUserMultipleSessions":
				ps.Model.AllowUserMultipleSessions = valStr == "true"
			case "ScheduledEventTimeZoneCode":
				ps.Model.ScheduledEventTimeZoneCode = valStr
			default:
				return mdlerrors.NewUnsupported("unknown model setting: " + key)
			}
		}

	case "language":
		if ps.Language == nil {
			return mdlerrors.NewNotFound("settings section", "language")
		}
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			switch key {
			case "DefaultLanguageCode":
				ps.Language.DefaultLanguageCode = valStr
			default:
				return mdlerrors.NewUnsupported("unknown language setting: " + key)
			}
		}

	case "workflows":
		if ps.Workflows == nil {
			return mdlerrors.NewNotFound("settings section", "workflows")
		}
		for key, val := range stmt.Properties {
			valStr := settingsValueToString(val)
			switch key {
			case "UserEntity":
				ps.Workflows.UserEntity = valStr
			case "DefaultTaskParallelism":
				if v, err := strconv.Atoi(valStr); err == nil {
					ps.Workflows.DefaultTaskParallelism = v
				}
			case "WorkflowEngineParallelism":
				if v, err := strconv.Atoi(valStr); err == nil {
					ps.Workflows.WorkflowEngineParallelism = v
				}
			default:
				return mdlerrors.NewUnsupported("unknown workflow setting: " + key)
			}
		}

	case "configuration":
		return alterSettingsConfiguration(ctx, ps, stmt)

	case "constant":
		return alterSettingsConstant(ctx, ps, stmt)

	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("unknown settings section: %s (expected model, configuration, constant, LANGUAGE, or workflows)", section))
	}

	// Write updated settings
	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	fmt.Fprintf(ctx.Output, "Updated %s settings\n", section)
	return nil
}

func alterSettingsConfiguration(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	// Find the named configuration
	var cfg *model.ServerConfiguration
	for _, c := range ps.Configuration.Configurations {
		if strings.EqualFold(c.Name, stmt.ConfigName) {
			cfg = c
			break
		}
	}
	if cfg == nil {
		return mdlerrors.NewNotFound("configuration", stmt.ConfigName)
	}

	for key, val := range stmt.Properties {
		valStr := settingsValueToString(val)
		switch key {
		case "DatabaseType":
			cfg.DatabaseType = valStr
		case "DatabaseUrl":
			cfg.DatabaseUrl = valStr
		case "DatabaseName":
			cfg.DatabaseName = valStr
		case "DatabaseUserName":
			cfg.DatabaseUserName = valStr
		case "DatabasePassword":
			cfg.DatabasePassword = valStr
		case "HttpPortNumber":
			if v, err := strconv.Atoi(valStr); err == nil {
				cfg.HttpPortNumber = v
			}
		case "ServerPortNumber":
			if v, err := strconv.Atoi(valStr); err == nil {
				cfg.ServerPortNumber = v
			}
		case "ApplicationRootUrl":
			cfg.ApplicationRootUrl = valStr
		default:
			return mdlerrors.NewUnsupported("unknown configuration setting: " + key)
		}
	}

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	fmt.Fprintf(ctx.Output, "Updated configuration '%s'\n", stmt.ConfigName)
	return nil
}

func alterSettingsConstant(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	// Find the target configuration
	targetConfig := stmt.ConfigName
	if targetConfig == "" {
		// Default to first configuration
		if len(ps.Configuration.Configurations) > 0 {
			targetConfig = ps.Configuration.Configurations[0].Name
		} else {
			return mdlerrors.NewValidation("no configurations found")
		}
	}

	var cfg *model.ServerConfiguration
	for _, c := range ps.Configuration.Configurations {
		if strings.EqualFold(c.Name, targetConfig) {
			cfg = c
			break
		}
	}
	if cfg == nil {
		return mdlerrors.NewNotFound("configuration", targetConfig)
	}

	if stmt.DropConstant {
		// Remove the constant override
		for i, cv := range cfg.ConstantValues {
			if cv.ConstantId == stmt.ConstantId {
				cfg.ConstantValues = append(cfg.ConstantValues[:i], cfg.ConstantValues[i+1:]...)
				if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
					return mdlerrors.NewBackend("update project settings", err)
				}
				fmt.Fprintf(ctx.Output, "Dropped constant '%s' from configuration '%s'\n",
					stmt.ConstantId, targetConfig)
				return nil
			}
		}
		return mdlerrors.NewNotFoundMsg("constant", stmt.ConstantId, fmt.Sprintf("constant '%s' not found in configuration '%s'", stmt.ConstantId, targetConfig))
	}

	// Find or create the constant value
	found := false
	for _, cv := range cfg.ConstantValues {
		if cv.ConstantId == stmt.ConstantId {
			cv.Value = stmt.Value
			found = true
			break
		}
	}
	if !found {
		cv := &model.ConstantValue{
			ConstantId: stmt.ConstantId,
			Value:      stmt.Value,
		}
		cv.TypeName = "Settings$ConstantValue"
		cfg.ConstantValues = append(cfg.ConstantValues, cv)
	}

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	fmt.Fprintf(ctx.Output, "Updated constant '%s' = '%s' in configuration '%s'\n",
		stmt.ConstantId, stmt.Value, targetConfig)
	return nil
}

// createConfiguration handles CREATE CONFIGURATION 'name' [properties...].
func createConfiguration(ctx *ExecContext, stmt *ast.CreateConfigurationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	// Check if configuration already exists
	for _, cfg := range ps.Configuration.Configurations {
		if strings.EqualFold(cfg.Name, stmt.Name) {
			return mdlerrors.NewAlreadyExists("configuration", stmt.Name)
		}
	}

	newCfg := &model.ServerConfiguration{
		Name:           stmt.Name,
		DatabaseType:   "HSQLDB",
		HttpPortNumber: 8080,
		ConstantValues: []*model.ConstantValue{},
	}
	newCfg.TypeName = "Settings$ServerConfiguration"

	// Apply optional properties
	for key, val := range stmt.Properties {
		valStr := settingsValueToString(val)
		switch key {
		case "DatabaseType":
			newCfg.DatabaseType = valStr
		case "DatabaseUrl":
			newCfg.DatabaseUrl = valStr
		case "DatabaseName":
			newCfg.DatabaseName = valStr
		case "DatabaseUserName":
			newCfg.DatabaseUserName = valStr
		case "DatabasePassword":
			newCfg.DatabasePassword = valStr
		case "HttpPortNumber":
			if v, err := strconv.Atoi(valStr); err == nil {
				newCfg.HttpPortNumber = v
			}
		case "ServerPortNumber":
			if v, err := strconv.Atoi(valStr); err == nil {
				newCfg.ServerPortNumber = v
			}
		case "ApplicationRootUrl":
			newCfg.ApplicationRootUrl = valStr
		default:
			return mdlerrors.NewUnsupported("unknown configuration property: " + key)
		}
	}

	ps.Configuration.Configurations = append(ps.Configuration.Configurations, newCfg)

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update project settings", err)
	}

	fmt.Fprintf(ctx.Output, "Created configuration: %s\n", stmt.Name)
	return nil
}

// dropConfiguration handles DROP CONFIGURATION 'name'.
func dropConfiguration(ctx *ExecContext, stmt *ast.DropConfigurationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}

	if ps.Configuration == nil {
		return mdlerrors.NewNotFound("settings section", "configuration")
	}

	for i, cfg := range ps.Configuration.Configurations {
		if strings.EqualFold(cfg.Name, stmt.Name) {
			ps.Configuration.Configurations = append(
				ps.Configuration.Configurations[:i],
				ps.Configuration.Configurations[i+1:]...,
			)
			if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
				return mdlerrors.NewBackend("update project settings", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped configuration: %s\n", stmt.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("configuration", stmt.Name)
}

// settingsValueToString converts an AST settings value to string.
func settingsValueToString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}
