// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: test_write <mpr-path>")
		os.Exit(1)
	}

	mprPath := os.Args[1]

	// Create a backup first
	backupPath := mprPath + ".backup"
	fmt.Printf("Creating backup: %s\n", backupPath)
	if err := copyFile(mprPath, backupPath); err != nil {
		fmt.Printf("Warning: Could not create backup: %v\n", err)
	}

	// Also backup mprcontents
	contentsDir := filepath.Join(filepath.Dir(mprPath), "mprcontents")
	backupContentsDir := contentsDir + ".backup"
	if _, err := os.Stat(contentsDir); err == nil {
		fmt.Printf("Creating mprcontents backup: %s\n", backupContentsDir)
		cmd := exec.Command("cp", "-r", contentsDir, backupContentsDir)
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: Could not backup mprcontents: %v\n", err)
		}
	}

	// Open writer
	fmt.Println("\nOpening writer...")
	writer, err := mpr.NewWriter(mprPath)
	if err != nil {
		fmt.Printf("Error opening writer: %v\n", err)
		os.Exit(1)
	}
	defer writer.Close()

	reader := writer.Reader()
	fmt.Printf("MPR Version: %d\n", reader.Version())

	// List existing modules to find one to add entity to
	modules, err := reader.ListModules()
	if err != nil {
		fmt.Printf("Error listing modules: %v\n", err)
		os.Exit(1)
	}

	if len(modules) == 0 {
		fmt.Println("No modules found")
		os.Exit(1)
	}

	// Find a module that's not from AppStore
	var targetModule *model.Module
	for _, m := range modules {
		if !m.FromAppStore {
			targetModule = m
			break
		}
	}
	if targetModule == nil {
		targetModule = modules[0]
	}

	fmt.Printf("\nTarget module: %s (ID: %s)\n", targetModule.Name, targetModule.ID)

	// Get the domain model for this module
	domainModels, err := reader.ListDomainModels()
	if err != nil {
		fmt.Printf("Error listing domain models: %v\n", err)
		os.Exit(1)
	}

	var targetDM *domainmodel.DomainModel
	for _, dm := range domainModels {
		if dm.ContainerID == targetModule.ID {
			targetDM = dm
			break
		}
	}

	if targetDM == nil {
		fmt.Printf("No domain model found for module %s\n", targetModule.Name)
		os.Exit(1)
	}

	fmt.Printf("Found domain model: %s\n", targetDM.ID)
	fmt.Printf("  Existing entities: %d\n", len(targetDM.Entities))

	// Create a new entity
	newEntity := &domainmodel.Entity{
		BaseElement: model.BaseElement{
			TypeName: "DomainModels$Entity",
		},
		Name:        "TestEntityFromGo",
		Persistable: true,
		Location: model.Point{
			X: 100,
			Y: 200,
		},
		Attributes: []*domainmodel.Attribute{
			{
				BaseElement: model.BaseElement{
					TypeName: "DomainModels$Attribute",
				},
				Name: "TestAttribute",
				Type: &domainmodel.StringAttributeType{Length: 200},
			},
		},
	}

	fmt.Printf("\nCreating entity: %s\n", newEntity.Name)
	err = writer.CreateEntity(targetDM.ID, newEntity)
	if err != nil {
		fmt.Printf("Error creating entity: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Entity created with ID: %s\n", newEntity.ID)

	// Verify by re-reading
	fmt.Println("\nVerifying by re-reading domain model...")
	updatedDM, err := reader.GetDomainModel(targetModule.ID)
	if err != nil {
		fmt.Printf("Error re-reading domain model: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Entities after creation: %d\n", len(updatedDM.Entities))
	for _, e := range updatedDM.Entities {
		if e.Name == newEntity.Name {
			fmt.Printf("  Found new entity: %s (ID: %s)\n", e.Name, e.ID)
			fmt.Printf("    Attributes: %d\n", len(e.Attributes))
			for _, a := range e.Attributes {
				fmt.Printf("      - %s\n", a.Name)
			}
		}
	}

	fmt.Println("\nTest completed successfully!")
	fmt.Println("\nNote: Restore from backup if needed:")
	fmt.Printf("  mv %s %s\n", backupPath, mprPath)
	fmt.Printf("  rm -rf %s && mv %s %s\n", contentsDir, backupContentsDir, contentsDir)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
