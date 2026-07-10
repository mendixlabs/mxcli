// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

func (b *Builder) buildNavigation() error {
	nav, err := b.reader.GetNavigation()
	if err != nil {
		return nil // Navigation may not exist
	}
	if nav == nil || len(nav.Profiles) == 0 {
		return nil
	}

	profileStmt, err := b.tx.Prepare(`
		INSERT INTO navigation_profiles_data (ProfileName, Kind, IsNative,
			HomePage, HomePageType, LoginPage, NotFoundPage,
			MenuItemCount, RoleBasedHomeCount, OfflineEntityCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer profileStmt.Close()

	menuStmt, err := b.tx.Prepare(`
		INSERT INTO navigation_menu_items (ProfileName, ItemPath, Depth, Caption,
			ActionType, TargetPage, TargetMicroflow, SubItemCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer menuStmt.Close()

	roleHomeStmt, err := b.tx.Prepare(`
		INSERT INTO navigation_role_homes (ProfileName, UserRole, Page, Microflow,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer roleHomeStmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	profileCount := 0
	menuCount := 0
	roleHomeCount := 0

	for _, profile := range nav.Profiles {
		// Determine home page info
		homePage := ""
		homePageType := ""
		if profile.HomePage != nil {
			if profile.HomePage.Page != "" {
				homePage = profile.HomePage.Page
				homePageType = "PAGE"
			} else if profile.HomePage.Microflow != "" {
				homePage = profile.HomePage.Microflow
				homePageType = "MICROFLOW"
			}
		}

		// Count menu items recursively
		totalMenuItems := countMenuItems(profile.MenuItems)

		isNative := 0
		if profile.IsNative {
			isNative = 1
		}

		_, err = profileStmt.Exec(
			profile.Name,
			profile.Kind,
			isNative,
			homePage,
			homePageType,
			profile.LoginPage,
			profile.NotFoundPage,
			totalMenuItems,
			len(profile.RoleBasedHomePages),
			len(profile.OfflineEntities),
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
		profileCount++

		// Insert menu items
		menuCount += insertMenuItems(menuStmt, profile.Name, profile.MenuItems, "", 0, projectID, snapshotID)

		// Insert role-based home pages
		for _, rh := range profile.RoleBasedHomePages {
			_, err = roleHomeStmt.Exec(
				profile.Name,
				rh.UserRole,
				rh.Page,
				rh.Microflow,
				projectID, snapshotID,
			)
			if err == nil {
				roleHomeCount++
			}
		}
	}

	b.report("Navigation profiles", profileCount)
	if menuCount > 0 {
		b.report("Navigation menu items", menuCount)
	}
	if roleHomeCount > 0 {
		b.report("Navigation role homes", roleHomeCount)
	}

	return nil
}

// countMenuItems recursively counts all menu items.
func countMenuItems(items []*types.NavMenuItem) int {
	count := len(items)
	for _, item := range items {
		count += countMenuItems(item.Items)
	}
	return count
}

// insertMenuItems recursively inserts menu items with hierarchical path encoding.
func insertMenuItems(stmt *sql.Stmt, profileName string, items []*types.NavMenuItem, parentPath string, depth int, projectID, snapshotID string) int {
	count := 0
	for i, item := range items {
		itemPath := fmt.Sprintf("%d", i)
		if parentPath != "" {
			itemPath = parentPath + "." + itemPath
		}

		_, err := stmt.Exec(
			profileName,
			itemPath,
			depth,
			item.Caption,
			item.ActionType,
			item.Page,
			item.Microflow,
			len(item.Items),
			projectID, snapshotID,
		)
		if err == nil {
			count++
		}

		if len(item.Items) > 0 {
			count += insertMenuItems(stmt, profileName, item.Items, itemPath, depth+1, projectID, snapshotID)
		}
	}
	return count
}
