// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// communitiesPopulated reports whether the graph community tables have been built
// (via REFRESH CATALOG COMMUNITIES). The community/cycle/layer/centrality tables
// are not populated by a plain full refresh.
func communitiesPopulated(ctx *ExecContext) bool {
	res, err := ctx.Catalog.Query("SELECT COUNT(*) FROM communities_data")
	if err != nil || len(res.Rows) == 0 {
		return false
	}
	if v, ok := res.Rows[0][0].(int64); ok {
		return v > 0
	}
	return false
}

func errNoCommunities() error {
	return mdlerrors.NewValidation("no community data — run 'refresh catalog communities' first")
}

// execShowCommunities handles SHOW COMMUNITIES — the community_summary listing.
func execShowCommunities(ctx *ExecContext) error {
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}
	if !communitiesPopulated(ctx) {
		return errNoCommunities()
	}
	res, err := ctx.Catalog.Query(
		"SELECT CommunityId, Label, Size, Modules FROM community_summary ORDER BY Size DESC")
	if err != nil {
		return mdlerrors.NewBackend("query communities", err)
	}
	fmt.Fprintf(ctx.Output, "Found %d communit%s\n", res.Count, plural(res.Count, "y", "ies"))
	outputCatalogResults(ctx, res)
	return nil
}

// execShowCommunity handles SHOW COMMUNITY [MEMBERS] OF Module.Asset.
func execShowCommunity(ctx *ExecContext, s *ast.ShowStmt, members bool) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("target name required for show community")
	}
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}
	if !communitiesPopulated(ctx) {
		return errNoCommunities()
	}
	name := s.Name.String()
	lit := "'" + strings.ReplaceAll(name, "'", "''") + "'"

	idRes, err := ctx.Catalog.Query("SELECT CommunityId FROM communities_data WHERE AssetName = " + lit + " LIMIT 1")
	if err != nil {
		return mdlerrors.NewBackend("query community", err)
	}
	if idRes.Count == 0 {
		fmt.Fprintf(ctx.Output, "%s is not in any community (not in the graph, or run 'refresh catalog communities')\n", name)
		return nil
	}
	cid := fmt.Sprintf("%v", idRes.Rows[0][0])

	if members {
		res, err := ctx.Catalog.Query(
			"SELECT AssetName, ModuleName FROM communities_data WHERE CommunityId = " + cid + " ORDER BY AssetName")
		if err != nil {
			return mdlerrors.NewBackend("query community members", err)
		}
		fmt.Fprintf(ctx.Output, "Community %s — %d member(s)\n", cid, res.Count)
		outputCatalogResults(ctx, res)
		return nil
	}

	res, err := ctx.Catalog.Query(
		"SELECT CommunityId, Label, Size, Modules FROM community_summary WHERE CommunityId = " + cid)
	if err != nil {
		return mdlerrors.NewBackend("query community", err)
	}
	fmt.Fprintf(ctx.Output, "%s is in community %s — use 'show community members of %s' for the full list\n", name, cid, name)
	outputCatalogResults(ctx, res)
	return nil
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}
