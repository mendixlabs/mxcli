// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// odataEnumerations creates, once per import, the enumeration an OData
// EnumType becomes (ako/mxcli#2477). Measured with mx check 11.12.6 on an
// external entity whose Status property is a vendor EnumType:
//
//	attribute with a plain enumeration     → CE4597 "This attribute has type
//	                                         'M.OrderStatus', but should have an
//	                                         enumeration type that was generated
//	                                         from 'Vendor.Sales.OrderStatus' in
//	                                         service 'M.Sales'."
//	enumeration with Rest$ODataRemoteEnumerationSource (service, qualified
//	type name) and a Rest$ODataRemoteEnumerationValue (member name) per value
//	                                       → 0 errors
//
// The enumeration is named after the EnumType, in the module the entities go
// to, one value per member with the member's name as name and caption.
type odataEnumerations struct {
	byRemote map[string]string // remote type name -> enumeration qualified name
	// deferred holds back creating an enumeration until createPending names
	// it: the single `create external entity` resolves every property and
	// keeps only the ones the statement lists (ako/mxcli#2497).
	deferred bool
	pending  map[string]*model.Enumeration // qualified name -> not yet created
}

// createPending creates the held-back enumeration of that qualified name, if
// there is one.
func (o *odataEnumerations) createPending(ctx *ExecContext, qn string) error {
	enum := o.pending[qn]
	if enum == nil {
		return nil
	}
	delete(o.pending, qn)
	if err := ctx.Backend.CreateEnumeration(enum); err != nil {
		return fmt.Errorf("creating enumeration %s: %w", qn, err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "  Created enumeration %s from %s\n", qn, enum.RemoteName)
	return nil
}

var enumValueName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ensure returns the qualified name of the enumeration for the remote type,
// creating it when the module has none; why is non-empty when the property
// cannot be imported, and says why.
func (o *odataEnumerations) ensure(ctx *ExecContext, moduleID model.ID, module, service, remoteType string, et *types.EdmEnumType) (qn, why string) {
	if qn, ok := o.byRemote[remoteType]; ok {
		return qn, ""
	}
	qn = module + "." + et.Name
	if existing := findEnumeration(ctx, module, et.Name); existing != nil {
		if existing.RemoteService == service && existing.RemoteName == remoteType {
			o.byRemote[remoteType] = qn
			return qn, ""
		}
		return "", fmt.Sprintf("enumeration %s already exists and was not generated from %s in %s; "+
			"rename it, or import into another module, and run the import again", qn, remoteType, service)
	}
	// Rest$ODataRemoteEnumerationValue is in the metamodel from 10.12 only
	// (the source from 10.2); how an older project stores a generated value
	// was not measured, so it is not guessed.
	if pv := ctx.Backend.ProjectVersion(); pv != nil && !pv.IsAtLeast(10, 12) {
		return "", "importing an enum-typed property needs Mendix 10.12 or later (Rest$ODataRemoteEnumerationValue)"
	}
	if len(et.Members) == 0 {
		return "", "the enum type has no members"
	}
	enum := &model.Enumeration{
		ContainerID:   moduleID,
		Name:          et.Name,
		RemoteService: service,
		RemoteName:    remoteType,
	}
	for _, m := range et.Members {
		if !enumValueName.MatchString(m.Name) || types.PlatformReservedWords[strings.ToLower(m.Name)] {
			return "", fmt.Sprintf("member %q is not a valid enumeration value name", m.Name)
		}
		enum.Values = append(enum.Values, model.EnumerationValue{
			Name:       m.Name,
			Caption:    &model.Text{Translations: map[string]string{authoringLanguage(ctx): m.Name}},
			RemoteName: m.Name,
		})
	}
	if o.deferred {
		if o.pending == nil {
			o.pending = map[string]*model.Enumeration{}
		}
		o.pending[qn] = enum
		o.byRemote[remoteType] = qn
		return qn, ""
	}
	if err := ctx.Backend.CreateEnumeration(enum); err != nil {
		return "", "creating enumeration " + qn + ": " + err.Error()
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "  Created enumeration %s from %s\n", qn, remoteType)
	o.byRemote[remoteType] = qn
	return qn, ""
}
