// SPDX-License-Identifier: Apache-2.0

// Package security provides types for Mendix project security configuration.
package security

import (
	"fmt"
	"unicode"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// ProjectSecurity represents the project-level security configuration.
type ProjectSecurity struct {
	model.BaseElement
	SecurityLevel      string          `json:"securityLevel"`
	AdminUserName      string          `json:"adminUserName"`
	AdminPassword      string          `json:"adminPassword"`
	AdminUserRole      string          `json:"adminUserRole"`
	CheckSecurity      bool            `json:"checkSecurity"`
	StrictMode         bool            `json:"strictMode"`
	StrictPageUrlCheck bool            `json:"strictPageUrlCheck"`
	EnableDemoUsers    bool            `json:"enableDemoUsers"`
	EnableGuestAccess  bool            `json:"enableGuestAccess"`
	GuestUserRole      string          `json:"guestUserRole,omitempty"`
	UserRoles          []*UserRole     `json:"userRoles,omitempty"`
	DemoUsers          []*DemoUser     `json:"demoUsers,omitempty"`
	PasswordPolicy     *PasswordPolicy `json:"passwordPolicy,omitempty"`
}

// UserRole represents an application-level user role that combines module roles.
type UserRole struct {
	model.BaseElement
	Name                    string   `json:"name"`
	Description             string   `json:"description,omitempty"`
	ModuleRoles             []string `json:"moduleRoles,omitempty"`
	ManageAllRoles          bool     `json:"manageAllRoles"`
	ManageUsersWithoutRoles bool     `json:"manageUsersWithoutRoles"`
	ManageableRoles         []string `json:"manageableRoles,omitempty"`
	CheckSecurity           bool     `json:"checkSecurity"`
}

// DemoUser represents a demo user for development/testing.
type DemoUser struct {
	model.BaseElement
	UserName  string   `json:"userName"`
	Password  string   `json:"password"`
	Entity    string   `json:"entity"`
	UserRoles []string `json:"userRoles,omitempty"`
}

// PasswordPolicy represents the password policy settings.
type PasswordPolicy struct {
	model.BaseElement
	MinimumLength    int  `json:"minimumLength"`
	RequireDigit     bool `json:"requireDigit"`
	RequireMixedCase bool `json:"requireMixedCase"`
	RequireSymbol    bool `json:"requireSymbol"`
}

// ValidatePassword checks a password against the policy.
// Returns nil if the password is compliant, or an error describing the first violation.
// A nil policy accepts any password.
func (p *PasswordPolicy) ValidatePassword(password string) error {
	if p == nil {
		return nil
	}
	if p.MinimumLength > 0 && len(password) < p.MinimumLength {
		return fmt.Errorf("password must be at least %d characters (got %d)", p.MinimumLength, len(password))
	}
	if p.RequireDigit && !containsDigit(password) {
		return fmt.Errorf("password must contain at least one digit")
	}
	if p.RequireMixedCase && !containsMixedCase(password) {
		return fmt.Errorf("password must contain both uppercase and lowercase letters")
	}
	if p.RequireSymbol && !containsSymbol(password) {
		return fmt.Errorf("password must contain at least one symbol")
	}
	return nil
}

func containsDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func containsMixedCase(s string) bool {
	hasUpper, hasLower := false, false
	for _, r := range s {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
		if hasUpper && hasLower {
			return true
		}
	}
	return false
}

func containsSymbol(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// ModuleSecurity represents the security configuration for a module.
type ModuleSecurity struct {
	model.BaseElement
	ContainerID model.ID      `json:"containerId"`
	ModuleRoles []*ModuleRole `json:"moduleRoles,omitempty"`
}

// GetContainerID returns the ID of the containing module.
func (ms *ModuleSecurity) GetContainerID() model.ID {
	return ms.ContainerID
}

// ModuleRole represents a module-level security role.
type ModuleRole struct {
	model.BaseElement
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// GetName returns the module role's name.
func (mr *ModuleRole) GetName() string {
	return mr.Name
}

// SecurityLevel constants matching BSON SecurityLevel enum values.
const (
	SecurityLevelOff        = "CheckNothing"
	SecurityLevelPrototype  = "CheckFormsAndMicroflows"
	SecurityLevelProduction = "CheckEverything"
)

// SecurityLevelDisplay returns a human-friendly name for a security level.
func SecurityLevelDisplay(level string) string {
	switch level {
	case SecurityLevelOff:
		return "Off"
	case SecurityLevelPrototype:
		return "Prototype / demo"
	case SecurityLevelProduction:
		return "Production"
	default:
		return level
	}
}
