// SPDX-License-Identifier: Apache-2.0

package types

import "github.com/JordtenBulte-OLC/mxcli/model"

// NavigationDocument represents a parsed navigation document.
type NavigationDocument struct {
	model.BaseElement
	ContainerID model.ID             `json:"containerId"`
	Name        string               `json:"name"`
	Profiles    []*NavigationProfile `json:"profiles,omitempty"`
}

// GetName returns the navigation document's name.
func (nd *NavigationDocument) GetName() string { return nd.Name }

// GetContainerID returns the container ID.
func (nd *NavigationDocument) GetContainerID() model.ID { return nd.ContainerID }

// NavigationProfile represents a single navigation profile.
type NavigationProfile struct {
	Name               string              `json:"name"`
	Kind               string              `json:"kind"`
	IsNative           bool                `json:"isNative"`
	HomePage           *NavHomePage        `json:"homePage,omitempty"`
	RoleBasedHomePages []*NavRoleBasedHome `json:"roleBasedHomePages,omitempty"`
	LoginPage          string              `json:"loginPage,omitempty"`
	NotFoundPage       string              `json:"notFoundPage,omitempty"`
	MenuItems          []*NavMenuItem      `json:"menuItems,omitempty"`
	OfflineEntities    []*NavOfflineEntity `json:"offlineEntities,omitempty"`
}

// NavHomePage holds a profile's default home page.
type NavHomePage struct {
	Page      string `json:"page,omitempty"`
	Microflow string `json:"microflow,omitempty"`
}

// NavRoleBasedHome maps a user role to a home page.
type NavRoleBasedHome struct {
	UserRole  string `json:"userRole"`
	Page      string `json:"page,omitempty"`
	Microflow string `json:"microflow,omitempty"`
}

// NavMenuItem is a recursive navigation menu entry.
type NavMenuItem struct {
	Caption    string         `json:"caption"`
	Page       string         `json:"page,omitempty"`
	Microflow  string         `json:"microflow,omitempty"`
	ActionType string         `json:"actionType,omitempty"`
	Items      []*NavMenuItem `json:"items,omitempty"`
}

// NavOfflineEntity declares offline sync rules for an entity.
type NavOfflineEntity struct {
	Entity     string `json:"entity"`
	SyncMode   string `json:"syncMode"`
	Constraint string `json:"constraint,omitempty"`
}

// NavigationProfileSpec specifies changes to a navigation profile.
type NavigationProfileSpec struct {
	HomePages    []NavHomePageSpec
	LoginPage    string
	NotFoundPage string
	MenuItems    []NavMenuItemSpec
	HasMenu      bool
}

// NavHomePageSpec specifies a home page assignment.
type NavHomePageSpec struct {
	IsPage  bool
	Target  string
	ForRole string
}

// NavMenuItemSpec specifies a menu item (recursive).
type NavMenuItemSpec struct {
	Caption   string
	Page      string
	Microflow string
	Items     []NavMenuItemSpec
}
