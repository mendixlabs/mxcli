// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Pages use a SEPARATE write protocol (pg_patch_page), not PED — the PED tools
// are forbidden for pages. pg_patch_page takes a high-level widget tree (the
// "LightPage"); this file maps the executor's pages.Page (shell + LayoutCall
// slots + pages.Widget tree) onto that tree. Widget/action coverage grows one
// type at a time, like the microflow activities; unmapped widgets/actions are
// rejected with a clear error. See docs/03-development/PED_MCP_CAPABILITIES.md.
// (Studio Pro 11.12 replaced the older pg_write_page with pg_patch_page.)

// CreatePage creates a page via pg_patch_page (a root-replace patch). A foldered
// page (folder clause) is created at the module root: pg_patch_page takes no
// folderPath, so unlike the ped_* document creates a page can't be placed in a
// folder over MCP — the page is still created (addressable by its qualified
// name), just not foldered.
func (b *Backend) CreatePage(page *pages.Page) error {
	moduleName, _, err := b.resolveDocContainer(page.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve container for page %q: %w", page.Name, err)
	}

	layout := "Atlas_Core.Atlas_Default"
	if page.LayoutCall != nil && page.LayoutCall.LayoutName != "" {
		layout = page.LayoutCall.LayoutName
	}

	// Build the layout-slot content widgets. Each LayoutCall argument fills one
	// layout slot; pg_patch_page wraps each as a Pages$Content with a slot name.
	slotWidgets := make([]any, 0)
	if page.LayoutCall != nil {
		for i, arg := range page.LayoutCall.Arguments {
			if len(arg.Widgets) == 0 {
				continue
			}
			mapped := make([]any, 0, len(arg.Widgets))
			for _, aw := range arg.Widgets {
				w, err := b.mapPageWidget(aw)
				if err != nil {
					return fmt.Errorf("page %q: %w", page.Name, err)
				}
				mapped = append(mapped, w)
			}
			slotWidgets = append(slotWidgets, map[string]any{
				"$Type":   "Pages$Content",
				"slot":    slotName(arg, i),
				"widgets": mapped,
			})
		}
	}
	if len(slotWidgets) == 0 {
		return fmt.Errorf("page %q: no content widgets — an empty page is not supported by the MCP backend", page.Name)
	}

	content := map[string]any{
		"title":      textValue(page.Title),
		"layout":     layout,
		"parameters": pageParameters(page.Parameters),
		"variables":  []any{},
		"widgets":    slotWidgets,
	}

	if err := b.pgWritePage(moduleName, page.Name, content); err != nil {
		return err
	}
	if page.ID == "" {
		page.ID = model.ID("mcp~page~" + moduleName + "~" + page.Name)
	}
	b.sessionPages = append(b.sessionPages, page)
	return nil
}

// ListPages returns pages from the local reader merged with those created over
// MCP this session (for the executor's duplicate/role checks).
func (b *Backend) ListPages() ([]*pages.Page, error) {
	local, err := b.reader.ListPages()
	if err != nil {
		return nil, err
	}
	if len(b.sessionPages) == 0 {
		return local, nil
	}
	seen := make(map[string]bool, len(b.sessionPages))
	out := make([]*pages.Page, 0, len(local)+len(b.sessionPages))
	for _, p := range b.sessionPages {
		seen[string(p.ContainerID)+"."+p.Name] = true
		out = append(out, p)
	}
	for _, p := range local {
		if !seen[string(p.ContainerID)+"."+p.Name] {
			out = append(out, p)
		}
	}
	return out, nil
}

// DeletePage drops a page via deleteDocument: ped_delete_document
// (Studio Pro 11.16+).
func (b *Backend) DeletePage(id model.ID) error {
	page, err := b.GetPage(id)
	if err != nil {
		return fmt.Errorf("resolve page %s for DROP: %w", id, err)
	}
	modName, err := b.moduleNameForContainer(page.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for page %q: %w", page.Name, err)
	}
	return b.deleteDocument(pageDocType, modName, page.Name)
}

// Page-related reads delegate to the local reader (the executor's page builder
// resolves layouts/snippets through these; without them the layout won't
// resolve and the widget tree is dropped).
func (b *Backend) GetPage(id model.ID) (*pages.Page, error) {
	// Mirror ListPages / GetMicroflow: a page created this session is visible by ID
	// within the same run, before it is saved to disk.
	for _, p := range b.sessionPages {
		if p.ID == id {
			return p, nil
		}
	}
	return b.reader.GetPage(id)
}
func (b *Backend) ListLayouts() ([]*pages.Layout, error)        { return b.reader.ListLayouts() }
func (b *Backend) GetLayout(id model.ID) (*pages.Layout, error) { return b.reader.GetLayout(id) }
func (b *Backend) ListSnippets() ([]*pages.Snippet, error)      { return b.reader.ListSnippets() }
func (b *Backend) ListBuildingBlocks() ([]*pages.BuildingBlock, error) {
	return b.reader.ListBuildingBlocks()
}
func (b *Backend) ListPageTemplates() ([]*pages.PageTemplate, error) {
	return b.reader.ListPageTemplates()
}

// slotName resolves the layout-slot name for a layout-call argument. The
// executor carries only the placeholder ID; for the common single-slot Atlas
// layouts the main slot is "Main". (Multi-slot resolution from the layout's
// placeholders is future work.)
func slotName(_ *pages.LayoutCallArgument, index int) string {
	if index == 0 {
		return "Main"
	}
	return fmt.Sprintf("Slot%d", index)
}

// pageParameters maps page parameters onto pg PageParameter objects.
func pageParameters(params []*pages.PageParameter) []any {
	out := make([]any, 0, len(params))
	for _, p := range params {
		po := map[string]any{"$Type": "Pages$PageParameter", "name": p.Name, "isRequired": p.IsRequired}
		if p.EntityName != "" {
			// pg_patch_page wants the entity type as a nested parameterType element
			// (DataTypes$ObjectType.entity), not a flat `entity` field — a flat field
			// is ignored and the parameter degrades to DataTypes$UnknownType.
			po["parameterType"] = map[string]any{
				"$Type":  "DataTypes$ObjectType",
				"entity": p.EntityName,
			}
		}
		out = append(out, po)
	}
	return out
}

// pgTruncationSentinel is the placeholder Studio Pro substitutes for a node
// deeper than pg_read_page's depth limit.
const pgTruncationSentinel = "..."

// pgReadFullDepth is the depth requested so pg_read_page returns a page whole.
//
// Studio Pro 11.13 added a `depth` argument that defaults to 4 and replaces
// deeper nodes with pgTruncationSentinel. That default is far shallower than a
// real page: an ordinary Atlas page read at depth 4 collapses to
// {"widgets":[{"$Type":"Pages$Content","slot":"Main","widgets":["...","..."]}]}
// — 1KB of a 32KB page. Since ALTER PAGE is read-modify-replace-whole-page, a
// truncated read would be written straight back over the real content, so the
// read must ask for the whole tree. Real pages nest far shallower than this
// bound, and hasTruncationSentinel catches it if one ever does not.
const pgReadFullDepth = 1000

// pgReadPage reads a page's current high-level content tree via pg_read_page.
// The result is the same LightPage shape pg_patch_page accepts, so it round-trips
// for read-modify-write (ALTER PAGE).
func (b *Backend) pgReadPage(moduleName, pageName string) (map[string]any, error) {
	args := map[string]any{
		"moduleName": moduleName,
		"pageName":   pageName,
	}
	// Only 11.13+ accepts `depth`; older servers declare the tool
	// additionalProperties:false and would reject the whole call.
	if b.client.SupportsToolArg("pg_read_page", "depth") {
		args["depth"] = pgReadFullDepth
	}
	res, err := b.client.CallTool("pg_read_page", args)
	if err != nil {
		return nil, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return nil, fmt.Errorf("pg_read_page %s.%s: %s", moduleName, pageName, text)
	}
	var content map[string]any
	if err := json.Unmarshal([]byte(text), &content); err != nil {
		return nil, fmt.Errorf("pg_read_page %s.%s: parsing content: %w", moduleName, pageName, err)
	}
	// Guard, don't drop (ADR-0005). A truncated read is unusable for
	// read-modify-write: writing it back replaces real widgets with the
	// sentinel. Refuse the read rather than let a partial page reach a write.
	if hasTruncationSentinel(content) {
		return nil, fmt.Errorf(
			"pg_read_page %s.%s: Studio Pro returned a depth-truncated page (%q placeholders); "+
				"mxcli cannot safely modify a page it cannot read whole. Modify this page in "+
				"Studio Pro, and please report the page name — mxcli requests the full depth, "+
				"so a truncated read means this Studio Pro version truncates in a way mxcli "+
				"does not yet handle",
			moduleName, pageName, pgTruncationSentinel)
	}
	return content, nil
}

// hasTruncationSentinel reports whether a LightPage carries a depth-truncation
// placeholder, i.e. the string "..." standing where an element should be.
//
// It only matches the sentinel as an *array element*, which is where a dropped
// widget lands. A "..." that is a property value (a caption or a title reading
// "...") is legitimate page content and must not trip the guard.
func hasTruncationSentinel(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for _, e := range t {
			if hasTruncationSentinel(e) {
				return true
			}
		}
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && s == pgTruncationSentinel {
				return true
			}
			if hasTruncationSentinel(e) {
				return true
			}
		}
	}
	return false
}

// pgWritePage writes a whole page (create or full overwrite) via pg_patch_page.
//
// Studio Pro 11.12 removed pg_write_page in favour of pg_patch_page (RFC 6902
// JSON Patch). A whole-page write is a single root-replace patch
// ({op:"replace", path:"", value:<full LightPage>}); pg_patch_page creates the
// page if it does not exist yet, so this one call serves both CreatePage and the
// mutator's Save(). The `value` is the same LightPage content pg_write_page took,
// so the content builders are unchanged. Failures are reported as a non-"success"
// result text (like the old tool), not res.IsError.
func (b *Backend) pgWritePage(moduleName, pageName string, content any) error {
	args := map[string]any{
		"moduleName": moduleName,
		"pageName":   pageName,
		"patches": []any{
			map[string]any{"op": "replace", "path": "", "value": content},
		},
	}
	res, err := b.client.CallTool("pg_patch_page", args)
	if isTimeoutErr(err) {
		// The root-replace patch is idempotent (create-if-missing + full
		// replace), so a retry cannot double-apply — and Studio Pro frequently
		// applies the patch even though the response timed out (-32000).
		time.Sleep(timeoutVerifyDelay)
		res, err = b.client.CallTool("pg_patch_page", args)
		if isTimeoutErr(err) {
			return timeoutUnverified(err, fmt.Sprintf("page %s.%s", moduleName, pageName))
		}
		if err == nil {
			timeoutNotice(fmt.Sprintf("page %s.%s", moduleName, pageName))
		}
	}
	if err != nil {
		return err
	}
	text := pedStripReminder(res.Text)
	if res.IsError || !strings.Contains(strings.ToLower(text), "success") {
		return fmt.Errorf("pg_patch_page %s.%s: %s", moduleName, pageName, text)
	}
	b.markDirty(moduleName)
	return nil
}
