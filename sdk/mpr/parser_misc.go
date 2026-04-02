// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (r *Reader) resolveContents(unitID string, contents []byte) ([]byte, error) {
	// For MPR v1, contents are stored directly in the database
	if r.version == MPRVersionV1 {
		return contents, nil
	}

	// For MPR v2, check if contents is a reference to an external file
	// Contents might be empty or contain just a hash
	if len(contents) > 0 {
		// Check if it's actual BSON content (starts with length prefix)
		if len(contents) >= 4 {
			return contents, nil
		}
	}

	// Look for the external file in mprcontents
	externalPath := filepath.Join(r.contentsDir, unitID)
	if _, err := os.Stat(externalPath); err == nil {
		return os.ReadFile(externalPath)
	}

	// Try with common extensions
	for _, ext := range []string{".mxunit", ".json", ""} {
		path := filepath.Join(r.contentsDir, unitID+ext)
		if data, err := os.ReadFile(path); err == nil {
			return data, nil
		}
	}

	return contents, nil
}

// parseSnippet parses snippet contents from BSON.
func (r *Reader) parseSnippet(unitID, containerID string, contents []byte) (*pages.Snippet, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	snippet := &pages.Snippet{}
	snippet.ID = model.ID(unitID)
	snippet.TypeName = "Pages$Snippet"
	snippet.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		snippet.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		snippet.Documentation = doc
	}
	if entityID := extractID(raw["Entity"]); entityID != "" {
		snippet.EntityID = model.ID(entityID)
	}

	return snippet, nil
}

// parseJavaAction parses Java action contents from BSON.
func (r *Reader) parseJavaAction(unitID, containerID string, contents []byte) (*JavaAction, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	ja := &JavaAction{}
	ja.ID = model.ID(unitID)
	ja.TypeName = "JavaActions$JavaAction"
	ja.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		ja.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		ja.Documentation = doc
	}

	return ja, nil
}

// extractID extracts an ID from various BSON representations.
// IDs in Mendix BSON can be strings, binary UUIDs, or nested structures.
func extractID(v any) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return blobToUUID(val)
	case map[string]any:
		// Could be a reference structure with $ID
		if id, ok := val["$ID"].(string); ok {
			return id
		}
		if id, ok := val["$ID"].([]byte); ok {
			return blobToUUID(id)
		}
	}

	return ""
}

// WriteJSON serializes the given element to JSON.
func WriteJSON(element any) ([]byte, error) {
	return json.MarshalIndent(element, "", "  ")
}

// parseJavaScriptAction parses JavaScript action contents from BSON.
func (r *Reader) parseJavaScriptAction(unitID, containerID string, contents []byte) (*JavaScriptAction, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	jsa := &JavaScriptAction{}
	jsa.ID = model.ID(unitID)
	jsa.TypeName = "JavaScriptActions$JavaScriptAction"
	jsa.ContainerID = model.ID(containerID)

	// Basic fields
	jsa.Name = extractString(raw["Name"])
	jsa.Documentation = extractString(raw["Documentation"])
	jsa.Platform = extractString(raw["Platform"])
	jsa.Excluded = extractBool(raw["Excluded"], false)
	jsa.ExportLevel = extractString(raw["ExportLevel"])
	jsa.ActionDefaultReturnName = extractString(raw["ActionDefaultReturnName"])

	// Parse return type
	switch rt := raw["JavaReturnType"].(type) {
	case map[string]any:
		jsa.ReturnType = parseCodeActionReturnType(rt)
	case primitive.D:
		jsa.ReturnType = parseCodeActionReturnType(primitiveToMap(rt))
	}

	// Parse parameters
	switch params := raw["Parameters"].(type) {
	case []any:
		for _, p := range params {
			if pMap := toMap(p); pMap != nil {
				if param := parseJavaActionParameter(pMap); param != nil {
					jsa.Parameters = append(jsa.Parameters, param)
				}
			}
		}
	case primitive.A:
		for _, p := range params {
			if pMap := toMap(p); pMap != nil {
				if param := parseJavaActionParameter(pMap); param != nil {
					jsa.Parameters = append(jsa.Parameters, param)
				}
			}
		}
	}

	// Parse type parameters
	switch typeParams := raw["TypeParameters"].(type) {
	case []any:
		for _, tp := range typeParams {
			if tpMap := toMap(tp); tpMap != nil {
				if name := extractString(tpMap["Name"]); name != "" {
					jsa.TypeParameters = append(jsa.TypeParameters, &javaactions.TypeParameterDef{
						BaseElement: model.BaseElement{ID: model.ID(extractBsonID(tpMap["$ID"]))},
						Name:        name,
					})
				}
			}
		}
	case primitive.A:
		for _, tp := range typeParams {
			if tpMap := toMap(tp); tpMap != nil {
				if name := extractString(tpMap["Name"]); name != "" {
					jsa.TypeParameters = append(jsa.TypeParameters, &javaactions.TypeParameterDef{
						BaseElement: model.BaseElement{ID: model.ID(extractBsonID(tpMap["$ID"]))},
						Name:        name,
					})
				}
			}
		}
	}

	// Parse MicroflowActionInfo
	if mai := toMap(raw["MicroflowActionInfo"]); mai != nil {
		jsa.MicroflowActionInfo = &javaactions.MicroflowActionInfo{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(mai["$ID"]))},
			Caption:     extractString(mai["Caption"]),
			Category:    extractString(mai["Category"]),
			Icon:        extractString(mai["Icon"]),
			ImageData:   extractString(mai["ImageData"]),
		}
	}

	// Resolve type parameter names for EntityTypeParameterType and TypeParameter
	for _, param := range jsa.Parameters {
		switch pt := param.ParameterType.(type) {
		case *javaactions.EntityTypeParameterType:
			pt.TypeParameterName = jsa.FindTypeParameterName(pt.TypeParameterID)
		case *javaactions.TypeParameter:
			if pt.TypeParameterID != "" && pt.TypeParameter == "" {
				pt.TypeParameter = jsa.FindTypeParameterName(pt.TypeParameterID)
			}
		}
	}

	// Resolve type parameter name for return type
	if tp, ok := jsa.ReturnType.(*javaactions.TypeParameter); ok {
		if tp.TypeParameterID != "" && tp.TypeParameter == "" {
			tp.TypeParameter = jsa.FindTypeParameterName(tp.TypeParameterID)
		}
	}

	return jsa, nil
}

// ReadJavaScriptActionByName reads a JavaScript action by qualified name (Module.ActionName).
func (r *Reader) ReadJavaScriptActionByName(qualifiedName string) (*JavaScriptAction, error) {
	units, err := r.listUnitsByType("JavaScriptActions$JavaScriptAction")
	if err != nil {
		return nil, err
	}

	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}
	moduleNames := make(map[model.ID]string)
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}

	folders, err := r.ListFolders()
	if err != nil {
		return nil, err
	}
	folderContainers := make(map[model.ID]model.ID)
	for _, f := range folders {
		folderContainers[f.ID] = f.ContainerID
	}

	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}

		var raw map[string]any
		if err := bson.Unmarshal(contents, &raw); err != nil {
			continue
		}

		name := extractString(raw["Name"])

		modName := ""
		containerID := model.ID(u.ContainerID)
		for range 20 {
			if mn, ok := moduleNames[containerID]; ok {
				modName = mn
				break
			}
			if parent, ok := folderContainers[containerID]; ok {
				containerID = parent
			} else {
				break
			}
		}

		if modName+"."+name == qualifiedName {
			return r.parseJavaScriptAction(u.ID, u.ContainerID, contents)
		}
	}

	return nil, fmt.Errorf("javascript action not found: %s", qualifiedName)
}

// parseBuildingBlock parses building block contents from BSON.
func (r *Reader) parseBuildingBlock(unitID, containerID string, contents []byte) (*pages.BuildingBlock, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	bb := &pages.BuildingBlock{}
	bb.ID = model.ID(unitID)
	bb.TypeName = "Forms$BuildingBlock"
	bb.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		bb.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		bb.Documentation = doc
	}

	return bb, nil
}

// parsePageTemplate parses page template contents from BSON.
func (r *Reader) parsePageTemplate(unitID, containerID string, contents []byte) (*pages.PageTemplate, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	pt := &pages.PageTemplate{}
	pt.ID = model.ID(unitID)
	pt.TypeName = "Forms$PageTemplate"
	pt.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		pt.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		pt.Documentation = doc
	}

	return pt, nil
}

// parseNavigationDocument parses navigation document contents from BSON.
func (r *Reader) parseNavigationDocument(unitID, containerID string, contents []byte) (*NavigationDocument, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	nav := &NavigationDocument{}
	nav.ID = model.ID(unitID)
	nav.TypeName = "Navigation$NavigationDocument"
	nav.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		nav.Name = name
	}

	// Parse navigation profiles
	for _, item := range extractBsonArray(raw["Profiles"]) {
		profMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		profile := parseNavigationProfile(profMap)
		if profile != nil {
			nav.Profiles = append(nav.Profiles, profile)
		}
	}

	return nav, nil
}

// parseNavigationProfile parses a single navigation profile from BSON.
func parseNavigationProfile(raw map[string]any) *NavigationProfile {
	typeName := extractString(raw["$Type"])
	profile := &NavigationProfile{
		Name: extractString(raw["Name"]),
		Kind: extractString(raw["Kind"]),
	}

	if typeName == "Navigation$NativeNavigationProfile" {
		profile.IsNative = true
		// Native home page
		if hp, ok := raw["NativeHomePage"].(map[string]any); ok {
			page := extractString(hp["HomePagePage"])
			nanoflow := extractString(hp["HomePageNanoflow"])
			if page != "" || nanoflow != "" {
				profile.HomePage = &NavHomePage{Page: page, Microflow: nanoflow}
			}
		}
		// Native role-based home pages
		for _, item := range extractBsonArray(raw["RoleBasedNativeHomePages"]) {
			if rbMap, ok := item.(map[string]any); ok {
				rbh := &NavRoleBasedHome{
					UserRole:  extractString(rbMap["UserRole"]),
					Page:      extractString(rbMap["HomePagePage"]),
					Microflow: extractString(rbMap["HomePageNanoflow"]),
				}
				if rbh.UserRole != "" {
					profile.RoleBasedHomePages = append(profile.RoleBasedHomePages, rbh)
				}
			}
		}
		// Native bottom bar items contribute to menu
		for _, item := range extractBsonArray(raw["BottomBarItems"]) {
			if barMap, ok := item.(map[string]any); ok {
				mi := parseNavMenuItemFromBottomBar(barMap)
				if mi != nil {
					profile.MenuItems = append(profile.MenuItems, mi)
				}
			}
		}
	} else {
		// Web profile (Navigation$NavigationProfile)
		// Default home page
		if hp, ok := raw["HomePage"].(map[string]any); ok {
			page := extractString(hp["Page"])
			mf := extractString(hp["Microflow"])
			if page != "" || mf != "" {
				profile.HomePage = &NavHomePage{Page: page, Microflow: mf}
			}
		}
		// Role-based home pages (stored as "HomeItems")
		for _, item := range extractBsonArray(raw["HomeItems"]) {
			if rbMap, ok := item.(map[string]any); ok {
				rbh := &NavRoleBasedHome{
					UserRole:  extractString(rbMap["UserRole"]),
					Page:      extractString(rbMap["Page"]),
					Microflow: extractString(rbMap["Microflow"]),
				}
				if rbh.UserRole != "" {
					profile.RoleBasedHomePages = append(profile.RoleBasedHomePages, rbh)
				}
			}
		}
		// Login page (stored as "LoginPageSettings" with type Forms$FormSettings)
		if lps, ok := raw["LoginPageSettings"].(map[string]any); ok {
			profile.LoginPage = extractString(lps["Form"])
		}
		// Not-found page
		if nfp, ok := raw["NotFoundHomepage"].(map[string]any); ok {
			profile.NotFoundPage = extractString(nfp["Page"])
			if profile.NotFoundPage == "" {
				profile.NotFoundPage = extractString(nfp["Microflow"])
			}
		}
		// Menu items (stored as "Menu" → MenuItemCollection)
		if menu, ok := raw["Menu"].(map[string]any); ok {
			for _, item := range extractBsonArray(menu["Items"]) {
				if miMap, ok := item.(map[string]any); ok {
					mi := parseNavMenuItem(miMap)
					if mi != nil {
						profile.MenuItems = append(profile.MenuItems, mi)
					}
				}
			}
		}
	}

	// Offline entity configs (both web and native)
	for _, item := range extractBsonArray(raw["OfflineEntityConfigs"]) {
		if oeMap, ok := item.(map[string]any); ok {
			oe := &NavOfflineEntity{
				Entity:     extractString(oeMap["Entity"]),
				SyncMode:   extractString(oeMap["SyncMode"]),
				Constraint: extractString(oeMap["Constraint"]),
			}
			if oe.Entity != "" {
				profile.OfflineEntities = append(profile.OfflineEntities, oe)
			}
		}
	}

	return profile
}

// parseNavMenuItem parses a Menus$MenuItem from BSON.
func parseNavMenuItem(raw map[string]any) *NavMenuItem {
	mi := &NavMenuItem{}

	// Extract caption text (Caption → Items → first Translation → Text)
	if caption, ok := raw["Caption"].(map[string]any); ok {
		mi.Caption = extractTextFromBson(caption)
	}

	// Extract action type and target from Action
	if action, ok := raw["Action"].(map[string]any); ok {
		actionType := extractString(action["$Type"])
		switch {
		case strings.HasSuffix(actionType, "FormAction") || strings.HasSuffix(actionType, "PageClientAction"):
			mi.ActionType = "PageAction"
			if fs, ok := action["FormSettings"].(map[string]any); ok {
				mi.Page = extractString(fs["Form"])
			}
		case strings.HasSuffix(actionType, "MicroflowAction") || strings.HasSuffix(actionType, "MicroflowClientAction"):
			mi.ActionType = "MicroflowAction"
			if ms, ok := action["MicroflowSettings"].(map[string]any); ok {
				mi.Microflow = extractString(ms["Microflow"])
			}
		case strings.HasSuffix(actionType, "OpenLinkAction") || strings.HasSuffix(actionType, "OpenLinkClientAction"):
			mi.ActionType = "OpenLinkAction"
		case strings.HasSuffix(actionType, "NoAction") || strings.HasSuffix(actionType, "NoClientAction"):
			mi.ActionType = "NoAction"
		default:
			mi.ActionType = actionType
		}
	}

	// Recurse into sub-items
	for _, item := range extractBsonArray(raw["Items"]) {
		if subMap, ok := item.(map[string]any); ok {
			sub := parseNavMenuItem(subMap)
			if sub != nil {
				mi.Items = append(mi.Items, sub)
			}
		}
	}

	// Only return if we have at least a caption or a page
	if mi.Caption == "" && mi.Page == "" && len(mi.Items) == 0 {
		return nil
	}
	return mi
}

// parseNavMenuItemFromBottomBar parses a NativePages$BottomBarItem as a NavMenuItem.
func parseNavMenuItemFromBottomBar(raw map[string]any) *NavMenuItem {
	mi := &NavMenuItem{}
	if caption, ok := raw["Caption"].(map[string]any); ok {
		mi.Caption = extractTextFromBson(caption)
	}
	mi.Page = extractString(raw["Page"])
	if mi.Caption == "" && mi.Page == "" {
		return nil
	}
	return mi
}

// extractTextFromBson extracts the first English text from a Texts$Text BSON object.
// Tries Items array first (Items → Translation → Text), then Translations map.
func extractTextFromBson(raw map[string]any) string {
	// Try Items array: [{LanguageCode: "en_US", Text: "..."}]
	for _, item := range extractBsonArray(raw["Items"]) {
		if transMap, ok := item.(map[string]any); ok {
			text := extractString(transMap["Text"])
			if text != "" {
				return text
			}
		}
	}
	// Try Translations array
	for _, item := range extractBsonArray(raw["Translations"]) {
		if transMap, ok := item.(map[string]any); ok {
			text := extractString(transMap["Text"])
			if text != "" {
				return text
			}
		}
	}
	return ""
}

// parseImageCollection parses image collection contents from BSON.
func (r *Reader) parseImageCollection(unitID, containerID string, contents []byte) (*ImageCollection, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	ic := &ImageCollection{}
	ic.ID = model.ID(unitID)
	ic.TypeName = "Images$ImageCollection"
	ic.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		ic.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		ic.Documentation = doc
	}
	if exp, ok := raw["ExportLevel"].(string); ok {
		ic.ExportLevel = exp
	}

	// Parse images in the collection
	if images, ok := raw["Images"].(bson.A); ok {
		for _, img := range images {
			if imgMap, ok := img.(map[string]any); ok {
				image := Image{}
				if id := extractID(imgMap["$ID"]); id != "" {
					image.ID = model.ID(id)
				}
				if name, ok := imgMap["Name"].(string); ok {
					image.Name = name
				}
				if format, ok := imgMap["ImageFormat"].(string); ok {
					image.Format = format
				}
				if data, ok := imgMap["Image"].(primitive.Binary); ok {
					image.Data = data.Data
				} else if data, ok := imgMap["Image"].([]byte); ok {
					image.Data = data
				}
				ic.Images = append(ic.Images, image)
			}
		}
	}

	return ic, nil
}

// parseJsonStructure parses JSON structure contents from BSON.
func (r *Reader) parseJsonStructure(unitID, containerID string, contents []byte) (*JsonStructure, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	js := &JsonStructure{}
	js.ID = model.ID(unitID)
	js.TypeName = "JsonStructures$JsonStructure"
	js.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		js.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		js.Documentation = doc
	}
	if snippet, ok := raw["JsonSnippet"].(string); ok {
		js.JsonSnippet = snippet
	}
	if exp, ok := raw["ExportLevel"].(string); ok {
		js.ExportLevel = exp
	}
	if exc, ok := raw["Excluded"].(bool); ok {
		js.Excluded = exc
	}

	// Parse elements (bson.A with version prefix)
	if elements, ok := raw["Elements"].(bson.A); ok {
		for _, elem := range elements {
			if elemMap, ok := elem.(map[string]any); ok {
				js.Elements = append(js.Elements, parseJsonElement(elemMap))
			}
		}
	}

	return js, nil
}

// parseJsonElement recursively parses a JsonStructures$JsonElement from BSON.
func parseJsonElement(raw map[string]any) *JsonElement {
	elem := &JsonElement{
		MaxLength:      -1,
		FractionDigits: -1,
		TotalDigits:    -1,
	}

	if v, ok := raw["ExposedName"].(string); ok {
		elem.ExposedName = v
	}
	if v, ok := raw["ExposedItemName"].(string); ok {
		elem.ExposedItemName = v
	}
	if v, ok := raw["Path"].(string); ok {
		elem.Path = v
	}
	if v, ok := raw["ElementType"].(string); ok {
		elem.ElementType = v
	}
	if v, ok := raw["PrimitiveType"].(string); ok {
		elem.PrimitiveType = v
	}
	if v, ok := raw["MinOccurs"].(int32); ok {
		elem.MinOccurs = int(v)
	}
	if v, ok := raw["MaxOccurs"].(int32); ok {
		elem.MaxOccurs = int(v)
	}
	if v, ok := raw["Nillable"].(bool); ok {
		elem.Nillable = v
	}
	if v, ok := raw["IsDefaultType"].(bool); ok {
		elem.IsDefaultType = v
	}
	if v, ok := raw["MaxLength"].(int32); ok {
		elem.MaxLength = int(v)
	}
	if v, ok := raw["FractionDigits"].(int32); ok {
		elem.FractionDigits = int(v)
	}
	if v, ok := raw["TotalDigits"].(int32); ok {
		elem.TotalDigits = int(v)
	}
	if v, ok := raw["OriginalValue"].(string); ok {
		elem.OriginalValue = v
	}

	// Parse children (bson.A with version prefix)
	if children, ok := raw["Children"].(bson.A); ok {
		for _, child := range children {
			if childMap, ok := child.(map[string]any); ok {
				elem.Children = append(elem.Children, parseJsonElement(childMap))
			}
		}
	}

	return elem
}
