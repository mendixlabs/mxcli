// SPDX-License-Identifier: Apache-2.0

// Example: Compare DataGrid2 CustomWidget BSON between two pages
// This helps identify differences in programmatically created vs Studio Pro created widgets
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func main() {
	mprPath := "/workspaces/ModelSDKGo/mx-test-projects/test2-go-app/test2-go.mpr"
	page1Name := "PgTest.P008_Product_Overview"   // Programmatically created
	page2Name := "PgTest.P008_Product_Overview_2" // Studio Pro fixed

	reader, err := mpr.Open(mprPath)
	if err != nil {
		fmt.Printf("Error opening MPR: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	// Get raw BSON for both pages
	page1, err := reader.GetRawUnitByName("page", page1Name)
	if err != nil {
		fmt.Printf("Error getting page1: %v\n", err)
		os.Exit(1)
	}

	page2, err := reader.GetRawUnitByName("page", page2Name)
	if err != nil {
		fmt.Printf("Error getting page2: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Comparing DataGrid2 CustomWidget BSON ===")
	fmt.Printf("Page 1: %s (programmatically created, has warning)\n", page1Name)
	fmt.Printf("Page 2: %s (Studio Pro fixed, no warning)\n\n", page2Name)

	// Parse BSON
	var raw1, raw2 map[string]any
	if err := bson.Unmarshal(page1.Contents, &raw1); err != nil {
		fmt.Printf("Error parsing page1 BSON: %v\n", err)
		os.Exit(1)
	}
	if err := bson.Unmarshal(page2.Contents, &raw2); err != nil {
		fmt.Printf("Error parsing page2 BSON: %v\n", err)
		os.Exit(1)
	}

	// Find CustomWidget in each page
	widget1 := findCustomWidget(raw1)
	widget2 := findCustomWidget(raw2)

	if widget1 == nil {
		fmt.Println("CustomWidget not found in page 1")
		os.Exit(1)
	}
	if widget2 == nil {
		fmt.Println("CustomWidget not found in page 2")
		os.Exit(1)
	}

	fmt.Println("=== Page 1 CustomWidget (programmatic) ===")
	printWidgetSummary(widget1, "")

	fmt.Println("\n=== Page 2 CustomWidget (Studio Pro) ===")
	printWidgetSummary(widget2, "")

	// Compare the Properties arrays in detail
	fmt.Println("\n=== Comparing Properties in Detail ===")
	obj1 := getMap(widget1, "Object")
	obj2 := getMap(widget2, "Object")
	props1 := getArray(obj1, "Properties")
	props2 := getArray(obj2, "Properties")

	// Convert to list of property maps for easier comparison
	propList1 := extractProperties(props1)
	propList2 := extractProperties(props2)

	fmt.Printf("Page 1 has %d properties\n", len(propList1))
	fmt.Printf("Page 2 has %d properties\n\n", len(propList2))

	// Compare each property by index (since they should be in same order)
	maxLen := max(len(propList2), len(propList1))

	// Hardcoded property names for DataGrid2 (from template analysis)
	propNames := []string{
		"advanced",                      // 0
		"datasource",                    // 1
		"refreshInterval",               // 2
		"itemSelection",                 // 3
		"itemSelectionMethod",           // 4
		"itemSelectionMode",             // 5
		"showSelectAllToggle",           // 6
		"keepSelection",                 // 7
		"loadingType",                   // 8
		"refreshIndicator",              // 9
		"columns",                       // 10
		"columnsFilterable",             // 11
		"pageSize",                      // 12
		"pagination",                    // 13
		"showPagingButtons",             // 14
		"showNumberOfRows",              // 15
		"pagingPosition",                // 16
		"loadMoreButtonCaption",         // 17
		"showEmptyPlaceholder",          // 18
		"emptyPlaceholder",              // 19
		"rowClass",                      // 20
		"onClickTrigger",                // 21
		"onClick",                       // 22
		"onSelectionChange",             // 23
		"filtersPlaceholder",            // 24
		"columnsSortable",               // 25
		"columnsResizable",              // 26
		"columnsDraggable",              // 27
		"columnsHidable",                // 28
		"configurationStorageType",      // 29
		"configurationAttribute",        // 30
		"storeFiltersInPersonalization", // 31
		"onConfigurationChange",         // 32
		"filterSectionTitle",            // 33
		"exportDialogLabel",             // 34
		"cancelExportLabel",             // 35
		"selectRowLabel",                // 36
		"selectAllRowsLabel",            // 37
		"selectedCountTemplateSingular", // 38
		"selectedCountTemplatePlural",   // 39
	}
	fmt.Printf("Using hardcoded property names for DataGrid2 (%d properties)\n", len(propNames))

	// Focus on meaningful differences (skip TypePointer/ID differences)
	fmt.Println("\n=== KEY DIFFERENCES (ignoring ID/pointer differences) ===")

	textTemplateDiffs := []int{}
	dataSourceDiffs := []int{}
	primitiveValueDiffs := []int{}

	for i := range maxLen {
		var p1, p2 map[string]any
		if i < len(propList1) {
			p1 = propList1[i]
		}
		if i < len(propList2) {
			p2 = propList2[i]
		}

		if p1 == nil || p2 == nil {
			fmt.Printf("Property %d: exists only in one page\n", i)
			continue
		}

		v1 := getMap(p1, "Value")
		v2 := getMap(p2, "Value")
		if v1 == nil || v2 == nil {
			continue
		}

		// Check TextTemplate
		tt1 := v1["TextTemplate"]
		tt2 := v2["TextTemplate"]
		if (tt1 == nil) != (tt2 == nil) {
			textTemplateDiffs = append(textTemplateDiffs, i)
		}

		// Check DataSource
		ds1 := getMap(v1, "DataSource")
		ds2 := getMap(v2, "DataSource")
		if ds1 != nil || ds2 != nil {
			if !compareDataSources(ds1, ds2) {
				dataSourceDiffs = append(dataSourceDiffs, i)
			}
		}

		// Check PrimitiveValue
		pv1, _ := v1["PrimitiveValue"].(string)
		pv2, _ := v2["PrimitiveValue"].(string)
		if pv1 != pv2 {
			primitiveValueDiffs = append(primitiveValueDiffs, i)
		}
	}

	// Report TextTemplate differences
	if len(textTemplateDiffs) > 0 {
		fmt.Println("=== TEXTTEMPLATE DIFFERENCES ===")
		fmt.Printf("Properties with TextTemplate null in Page 1 but populated in Page 2:\n\n")

		for _, i := range textTemplateDiffs {
			propName := "unknown"
			if i < len(propNames) {
				propName = propNames[i]
			}
			fmt.Printf("--- Property %d: %s ---\n", i, propName)
			p2 := propList2[i]
			v2 := getMap(p2, "Value")
			if v2 != nil {
				tt2 := v2["TextTemplate"]
				fmt.Printf("Page 1 TextTemplate: null\n")
				fmt.Printf("Page 2 TextTemplate:\n")
				prettyPrintIndent(tt2, "  ")
			}
		}
	}

	// Report DataSource differences
	if len(dataSourceDiffs) > 0 {
		fmt.Println("\n=== DATASOURCE DIFFERENCES ===")
		fmt.Printf("Properties with DataSource differences: %v\n\n", dataSourceDiffs)

		for _, i := range dataSourceDiffs {
			fmt.Printf("--- Property %d ---\n", i)
			p1 := propList1[i]
			p2 := propList2[i]
			v1 := getMap(p1, "Value")
			v2 := getMap(p2, "Value")
			if v1 != nil && v2 != nil {
				ds1 := getMap(v1, "DataSource")
				ds2 := getMap(v2, "DataSource")
				fmt.Println("Page 1 DataSource:")
				prettyPrintIndent(ds1, "  ")
				fmt.Println("Page 2 DataSource:")
				prettyPrintIndent(ds2, "  ")
			}
		}
	}

	// Report PrimitiveValue differences
	if len(primitiveValueDiffs) > 0 {
		fmt.Println("\n=== PRIMITIVEVALUE DIFFERENCES ===")
		for _, i := range primitiveValueDiffs {
			p1 := propList1[i]
			p2 := propList2[i]
			v1 := getMap(p1, "Value")
			v2 := getMap(p2, "Value")
			pv1, _ := v1["PrimitiveValue"].(string)
			pv2, _ := v2["PrimitiveValue"].(string)
			fmt.Printf("Property %d: Page1=%q, Page2=%q\n", i, pv1, pv2)
		}
	}

	if len(textTemplateDiffs) == 0 && len(dataSourceDiffs) == 0 && len(primitiveValueDiffs) == 0 {
		fmt.Println("No meaningful differences found (only ID/pointer differences)")
	}

	// Summary
	fmt.Println("\n\n========================================")
	fmt.Println("SUMMARY OF DIFFERENCES")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("The key difference between the programmatically created page and the")
	fmt.Println("Studio Pro fixed page is that several properties have null TextTemplate")
	fmt.Println("values in the programmatic version, but properly populated TextTemplate")
	fmt.Println("(Forms$ClientTemplate) objects in the Studio Pro version.")
	fmt.Println()
	fmt.Println("Properties with missing TextTemplate:")
	for _, i := range textTemplateDiffs {
		p2 := propList2[i]
		v2 := getMap(p2, "Value")
		if v2 != nil {
			tt2 := getMap(v2, "TextTemplate")
			if tt2 != nil {
				tmpl := getMap(tt2, "Template")
				if tmpl != nil {
					items := getArray(tmpl, "Items")
					for _, item := range items {
						m, ok := item.(map[string]any)
						if !ok {
							if d, ok := item.(bson.D); ok {
								m = bsonDToMap(d)
							}
						}
						if m != nil {
							if text, ok := m["Text"].(string); ok {
								propName := "unknown"
								if i < len(propNames) {
									propName = propNames[i]
								}
								fmt.Printf("  - Property %d (%s): has translation %q\n", i, propName, text)
							}
						}
					}
				}
			}
		}
	}
	fmt.Println()
	fmt.Println("The TextTemplate structure required is:")
	fmt.Println(`  Forms$ClientTemplate with:
    - $ID: unique GUID
    - $Type: "Forms$ClientTemplate"
    - Fallback: Texts$Text with empty Items
    - Parameters: [2] (version marker)
    - Template: Texts$Text with Items containing translations`)
	fmt.Println()
	fmt.Println("FIX: The widget writer code needs to create proper TextTemplate")
	fmt.Println("(Forms$ClientTemplate) structures for properties that have")
	fmt.Println("Type=TextTemplate in the widget definition, even when empty.")
}

// findCustomWidget recursively finds the first CustomWidget in the page structure
func findCustomWidget(obj any) map[string]any {
	switch v := obj.(type) {
	case map[string]any:
		if typeName, ok := v["$Type"].(string); ok {
			if typeName == "CustomWidgets$CustomWidget" {
				return v
			}
		}
		// Recursively search children
		for _, child := range v {
			if found := findCustomWidget(child); found != nil {
				return found
			}
		}
	case bson.D:
		m := bsonDToMap(v)
		return findCustomWidget(m)
	case bson.A:
		for _, item := range v {
			if found := findCustomWidget(item); found != nil {
				return found
			}
		}
	case []any:
		for _, item := range v {
			if found := findCustomWidget(item); found != nil {
				return found
			}
		}
	}
	return nil
}

func bsonDToMap(d bson.D) map[string]any {
	m := make(map[string]any)
	for _, elem := range d {
		m[elem.Key] = elem.Value
	}
	return m
}

func getMap(obj map[string]any, key string) map[string]any {
	if v, ok := obj[key]; ok {
		switch t := v.(type) {
		case map[string]any:
			return t
		case bson.D:
			return bsonDToMap(t)
		}
	}
	return nil
}

func getArray(obj map[string]any, key string) []any {
	if v, ok := obj[key]; ok {
		switch t := v.(type) {
		case []any:
			return t
		case bson.A:
			result := make([]any, len(t))
			for i, item := range t {
				result[i] = item
			}
			return result
		}
	}
	return nil
}

func printWidgetSummary(widget map[string]any, prefix string) {
	fmt.Printf("%s$Type: %v\n", prefix, widget["$Type"])
	if name, ok := widget["Name"].(string); ok {
		fmt.Printf("%sName: %s\n", prefix, name)
	}

	// Print Type info
	if typeObj := getMap(widget, "Type"); typeObj != nil {
		fmt.Printf("%sType:\n", prefix)
		fmt.Printf("%s  WidgetID: %v\n", prefix, typeObj["WidgetID"])
		fmt.Printf("%s  Name: %v\n", prefix, typeObj["Name"])
	}

	// Print Object summary
	if objObj := getMap(widget, "Object"); objObj != nil {
		fmt.Printf("%sObject:\n", prefix)
		fmt.Printf("%s  $Type: %v\n", prefix, objObj["$Type"])
		if props := getArray(objObj, "Properties"); props != nil {
			fmt.Printf("%s  Properties count: %d\n", prefix, len(props)-1) // -1 for version marker
		}
	}
}

func compareObjects(name string, obj1, obj2 map[string]any, depth int) {
	indent := strings.Repeat("  ", depth)

	if obj1 == nil && obj2 == nil {
		return
	}
	if obj1 == nil {
		fmt.Printf("%s%s: only in page 2\n", indent, name)
		return
	}
	if obj2 == nil {
		fmt.Printf("%s%s: only in page 1\n", indent, name)
		return
	}

	// Find all keys
	allKeys := make(map[string]bool)
	for k := range obj1 {
		allKeys[k] = true
	}
	for k := range obj2 {
		allKeys[k] = true
	}

	for key := range allKeys {
		v1, has1 := obj1[key]
		v2, has2 := obj2[key]

		if !has1 {
			fmt.Printf("%s%s.%s: only in page 2 = %v\n", indent, name, key, summarizeValue(v2))
			continue
		}
		if !has2 {
			fmt.Printf("%s%s.%s: only in page 1 = %v\n", indent, name, key, summarizeValue(v1))
			continue
		}

		// Both have the key, compare values
		if !reflect.DeepEqual(v1, v2) {
			// Skip $ID differences
			if key == "$ID" {
				continue
			}
			fmt.Printf("%s%s.%s differs:\n", indent, name, key)
			fmt.Printf("%s  Page 1: %v\n", indent, summarizeValue(v1))
			fmt.Printf("%s  Page 2: %v\n", indent, summarizeValue(v2))
		}
	}
}

func summarizeValue(v any) string {
	switch t := v.(type) {
	case string:
		if len(t) > 80 {
			return fmt.Sprintf("%q... (len=%d)", t[:80], len(t))
		}
		return fmt.Sprintf("%q", t)
	case []any:
		return fmt.Sprintf("[array len=%d]", len(t))
	case bson.A:
		return fmt.Sprintf("[bson.A len=%d]", len(t))
	case map[string]any:
		return fmt.Sprintf("{map keys=%d}", len(t))
	case bson.D:
		return fmt.Sprintf("{bson.D keys=%d}", len(t))
	default:
		return fmt.Sprintf("%v", v)
	}
}

func extractProperties(props []any) []map[string]any {
	var result []map[string]any
	for _, item := range props {
		switch t := item.(type) {
		case map[string]any:
			result = append(result, t)
		case bson.D:
			result = append(result, bsonDToMap(t))
		}
	}
	return result
}

func getTypePointerString(prop map[string]any) string {
	tp := prop["TypePointer"]
	if tp == nil {
		return "<nil>"
	}
	switch t := tp.(type) {
	case primitive.Binary:
		return base64.StdEncoding.EncodeToString(t.Data)
	case map[string]any:
		if data, ok := t["Data"].(primitive.Binary); ok {
			return base64.StdEncoding.EncodeToString(data.Data)
		}
	}
	return fmt.Sprintf("%v", tp)
}

func compareValues(v1, v2 map[string]any) []string {
	var diffs []string

	// Key fields to compare
	keysToCheck := []string{
		"PrimitiveValue", "Expression", "Form", "Microflow", "Nanoflow",
		"Image", "Selection", "XPathConstraint",
		"TextTemplate", "TranslatableValue", "Action", "DataSource",
		"EntityRef", "AttributeRef", "Icon", "SourceVariable",
	}

	for _, key := range keysToCheck {
		val1, has1 := v1[key]
		val2, has2 := v2[key]

		// Normalize nil checks
		isNil1 := !has1 || val1 == nil
		isNil2 := !has2 || val2 == nil

		if isNil1 && isNil2 {
			continue
		}

		if isNil1 != isNil2 {
			if isNil1 {
				diffs = append(diffs, fmt.Sprintf("%s: Page1=<nil>, Page2=%s", key, summarizeValue(val2)))
			} else {
				diffs = append(diffs, fmt.Sprintf("%s: Page1=%s, Page2=<nil>", key, summarizeValue(val1)))
			}
			continue
		}

		if !reflect.DeepEqual(val1, val2) {
			diffs = append(diffs, fmt.Sprintf("%s: Page1=%s, Page2=%s", key, summarizeValue(val1), summarizeValue(val2)))
		}
	}

	// Also check Objects and Widgets arrays
	obj1 := getArray(v1, "Objects")
	obj2 := getArray(v2, "Objects")
	if len(obj1) != len(obj2) {
		diffs = append(diffs, fmt.Sprintf("Objects count: Page1=%d, Page2=%d", len(obj1), len(obj2)))
	}

	w1 := getArray(v1, "Widgets")
	w2 := getArray(v2, "Widgets")
	if len(w1) != len(w2) {
		diffs = append(diffs, fmt.Sprintf("Widgets count: Page1=%d, Page2=%d", len(w1), len(w2)))
	}

	return diffs
}

func printPropertyDetail(prop map[string]any) {
	tp := getTypePointerString(prop)
	fmt.Printf("  TypePointer: %s\n", tp)

	if val := getMap(prop, "Value"); val != nil {
		if pv, ok := val["PrimitiveValue"].(string); ok && pv != "" {
			fmt.Printf("  PrimitiveValue: %q\n", pv)
		}
		if val["TextTemplate"] != nil {
			fmt.Println("  TextTemplate: present")
		}
		if val["DataSource"] != nil {
			fmt.Println("  DataSource: present")
		}
	}
}

func prettyPrint(obj any) {
	jsonBytes, _ := json.MarshalIndent(obj, "", "  ")
	s := string(jsonBytes)
	if len(s) > 20000 {
		fmt.Println(s[:20000] + "\n... (truncated)")
	} else {
		fmt.Println(s)
	}
}

func prettyPrintIndent(obj any, prefix string) {
	jsonBytes, _ := json.MarshalIndent(obj, prefix, "  ")
	fmt.Println(prefix + string(jsonBytes))
}

func compareDataSources(ds1, ds2 map[string]any) bool {
	if ds1 == nil && ds2 == nil {
		return true
	}
	if ds1 == nil || ds2 == nil {
		return false
	}

	// Compare key fields (ignore IDs)
	keys := []string{"$Type", "EntityRef", "XPathConstraint", "SortBar"}
	for _, key := range keys {
		v1 := ds1[key]
		v2 := ds2[key]
		if !compareNonID(v1, v2) {
			return false
		}
	}
	return true
}

func compareNonID(v1, v2 any) bool {
	if v1 == nil && v2 == nil {
		return true
	}
	if v1 == nil || v2 == nil {
		return false
	}

	// For strings, compare directly
	s1, ok1 := v1.(string)
	s2, ok2 := v2.(string)
	if ok1 && ok2 {
		return s1 == s2
	}

	// For maps, skip $ID fields
	m1, ok1 := v1.(map[string]any)
	m2, ok2 := v2.(map[string]any)
	if ok1 && ok2 {
		// Compare $Type
		if m1["$Type"] != m2["$Type"] {
			return false
		}
		return true // Simplified - just check type matches
	}

	return reflect.DeepEqual(v1, v2)
}

// loadPropertyNames loads property names from a widget template file
func loadPropertyNames(templatePath string) []string {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		fmt.Printf("Warning: Could not load template: %v\n", err)
		return nil
	}

	var template map[string]any
	if err := json.Unmarshal(data, &template); err != nil {
		fmt.Printf("Warning: Could not parse template: %v\n", err)
		return nil
	}

	// Get Properties array from Object
	obj, _ := template["Object"].(map[string]any)
	if obj == nil {
		return nil
	}

	props, _ := obj["Properties"].([]any)
	if props == nil {
		return nil
	}

	var names []string
	for _, p := range props {
		prop, _ := p.(map[string]any)
		if prop == nil {
			continue
		}
		key, _ := prop["Key"].(string)
		names = append(names, key)
	}
	return names
}
