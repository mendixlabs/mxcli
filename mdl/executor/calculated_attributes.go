// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// resolveCalculatedValue builds the DomainModels$CalculatedValue for an
// attribute declared `CALCULATED BY Module.Microflow`, and refuses the bindings
// Mendix rejects.
//
// Two things are decided here (#917):
//
//   - PassEntity, which says whether the microflow receives the owning entity.
//     It is derived from the signature rather than assumed: a microflow that
//     takes the entity gets true, a parameterless one false. Guessing either way
//     produces a binding mxbuild rejects.
//   - Whether the binding is valid at all. Mendix checks the signature at build
//     time — a parameter of the wrong entity is CE7247 "Microflow parameter 'X'
//     should be of type Module.Entity" — so a mismatch is refused here rather
//     than written and discovered at build. Same placement as the other
//     write-blocking rules (the #833 lesson: `check` alone is not enough,
//     because a script can skip it).
//
// A bare `CALCULATED` with no microflow is left unbound: that is the "calculated
// but not yet wired" state Studio Pro also allows.
func resolveCalculatedValue(ctx *ExecContext, mfName *ast.QualifiedName, entityQN, attrName string, attrType ast.DataType) (*domainmodel.AttributeValue, error) {
	value := &domainmodel.AttributeValue{Type: "CalculatedValue"}
	if mfName == nil {
		return value, nil
	}
	qn := mfName.String()

	mfID, err := resolveMicroflowByName(ctx, qn)
	if err != nil {
		return nil, mdlerrors.NewBackend(fmt.Sprintf("attribute '%s'", attrName), err)
	}
	value.MicroflowID = mfID
	value.MicroflowName = qn

	// A microflow created earlier in this same script is not readable yet, so
	// its signature cannot be checked. Bind it and let the build have the last
	// word rather than refusing something that may well be correct.
	mf := findMicroflowByQualifiedName(ctx, qn)
	if mf == nil {
		value.PassEntity = true
		return value, nil
	}

	switch len(mf.Parameters) {
	case 0:
		// Parameterless calculation: Mendix stores PassEntity=false.
		value.PassEntity = false
	case 1:
		obj, ok := mf.Parameters[0].Type.(*microflows.ObjectType)
		if !ok {
			return nil, mdlerrors.NewValidationf(
				"attribute '%s': calculation microflow '%s' takes a %s parameter; it must take the owning entity '%s' (or no parameter at all)",
				attrName, qn, mf.Parameters[0].Type.GetTypeName(), entityQN)
		}
		if obj.EntityQualifiedName != entityQN {
			return nil, mdlerrors.NewValidationf(
				"attribute '%s': calculation microflow '%s' takes a parameter of type '%s'; it must take the owning entity '%s' (mxbuild reports this as CE7247)",
				attrName, qn, obj.EntityQualifiedName, entityQN)
		}
		value.PassEntity = true
	default:
		return nil, mdlerrors.NewValidationf(
			"attribute '%s': calculation microflow '%s' takes %d parameters; a calculated attribute passes at most the owning entity '%s'",
			attrName, qn, len(mf.Parameters), entityQN)
	}

	if want := calculatedReturnTypeName(attrType); want != "" {
		got := "Void"
		if mf.ReturnType != nil {
			got = mf.ReturnType.GetTypeName()
		}
		if !returnTypeSatisfies(got, want) {
			return nil, mdlerrors.NewValidationf(
				"attribute '%s': calculation microflow '%s' returns %s; the attribute is %s, and Mendix requires the return type to match",
				attrName, qn, got, want)
		}
	}
	return value, nil
}

// returnTypeSatisfies reports whether a microflow returning got can calculate an
// attribute wanting want. Mendix treats Integer and Long as one family here —
// its own message is CE7247 "Microflow return type should be Integer/Long." —
// so an Integer attribute calculated by a Long-returning microflow builds
// clean and must not be refused. Every other pairing is exact; that is measured
// for Integer/Long/String and assumed (strictly) elsewhere, which errs toward
// refusing rather than writing something the build rejects.
func returnTypeSatisfies(got, want string) bool {
	if got == want {
		return true
	}
	intFamily := func(s string) bool { return s == "Integer" || s == "Long" }
	return intFamily(got) && intFamily(want)
}

// calculatedReturnTypeName is the microflow return type an attribute of this
// kind requires, or "" when the kind is not one a calculation can produce (the
// auto-* pseudo-types) and the check should be skipped.
func calculatedReturnTypeName(t ast.DataType) string {
	switch t.Kind {
	case ast.TypeString, ast.TypeInteger, ast.TypeLong, ast.TypeDecimal,
		ast.TypeBoolean, ast.TypeDateTime, ast.TypeDate, ast.TypeBinary,
		ast.TypeEnumeration:
		return t.Kind.String()
	default:
		return ""
	}
}

// findMicroflowByQualifiedName returns the live microflow with this qualified
// name, or nil. Excluded twins are skipped (#914).
func findMicroflowByQualifiedName(ctx *ExecContext, qualifiedName string) *microflows.Microflow {
	// Only the microflows of that local name are decoded: listing all of them
	// to pick one cost a decode of every microflow in the project per call,
	// and `create or modify microflow` makes one per statement
	// (ako/mxcli#2094).
	localName := qualifiedName[strings.LastIndex(qualifiedName, ".")+1:]
	all, err := microflowsNamed(ctx, localName)
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	mf, ok := pickLive(all,
		func(m *microflows.Microflow) bool {
			return h.GetQualifiedName(m.ContainerID, m.Name) == qualifiedName
		},
		func(m *microflows.Microflow) bool { return m.Excluded },
	)
	if !ok {
		return nil
	}
	return mf
}
