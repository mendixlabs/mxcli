// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

// userRoleTokenUsers names the stored documents whose XPath constraints name
// the user role by its token, `'[%UserRole_<role>%]'` (ako/mxcli#2529). Each
// becomes CE1613 "The selected user role '<role>' no longer exists." once the
// role is dropped — at a retrieve, a data source, an access rule or a
// workflow's targeting alike (measured for the token on 11.15.0, #1723). The
// token is matched as stored, case-sensitively, which is how mxbuild matches
// it; `[%UserRole_SpareTwo%]` does not name `Spare`, since the `%]` closes it.
//
// Could-not-establish is silence: an unreadable project names nobody.
func userRoleTokenUsers(ctx *ExecContext, role string) []string {
	if ctx == nil || ctx.Backend == nil || role == "" {
		return nil
	}
	units, err := ctx.Backend.ListRawUnitsByType("")
	if err != nil {
		return nil
	}
	token := []byte("[%UserRole_" + role + "%]")
	h, _ := getHierarchy(ctx)
	seen := map[string]bool{}
	var out []string
	for _, u := range units {
		if !bytes.Contains(u.Contents, token) {
			continue
		}
		var head struct {
			Name string `bson:"Name"`
		}
		_ = bson.Unmarshal(u.Contents, &head)
		label := tokenUnitLabel(u.Type, head.Name, string(u.ContainerID), h)
		if !seen[label] {
			seen[label] = true
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func tokenUnitLabel(typ, name, container string, h *ContainerHierarchy) string {
	kind := typ
	if i := strings.LastIndex(kind, "$"); i >= 0 {
		kind = kind[i+1:]
	}
	if typ == "DomainModels$DomainModel" {
		if h != nil {
			return "the access rules of module " + h.GetModuleName(h.FindModuleID(model.ID(container)))
		}
		return "an entity's access rules"
	}
	if name != "" && h != nil {
		return strings.ToLower(kind) + " " + h.GetQualifiedName(model.ID(container), name)
	}
	if name != "" {
		return strings.ToLower(kind) + " " + name
	}
	return "a " + strings.ToLower(kind)
}

// userRoleTokenDropRefusal is why `drop user role` refuses a role an XPath
// token still names.
func userRoleTokenDropRefusal(role string, users []string) string {
	shown := users
	if len(shown) > 5 {
		shown = append(append([]string(nil), users[:5]...), fmt.Sprintf("%d more", len(users)-5))
	}
	return fmt.Sprintf("user role '%s' is named by the XPath token '[%%UserRole_%s%%]' in %s: dropping it leaves "+
		"each constraint naming a role that no longer exists (CE1613). Change or remove those constraints first",
		role, role, strings.Join(shown, ", "))
}
