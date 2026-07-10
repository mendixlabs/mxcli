package property

import "github.com/JordtenBulte-OLC/mxcli/modelsdk/element"

// ByNameRef is a property that references another element by qualified name.
type ByNameRef[T element.Element] struct {
	propertyBase
	targetType string
	qname      string
}

func NewByNameRef[T element.Element](name, targetType string) *ByNameRef[T] {
	return &ByNameRef[T]{propertyBase: propertyBase{name: name}, targetType: targetType}
}

func (r *ByNameRef[T]) TargetType() string    { return r.targetType }
func (r *ByNameRef[T]) QualifiedName() string { return r.qname }
func (r *ByNameRef[T]) BSONValue() any        { return r.qname }

// SetQualifiedName sets the reference and marks the property dirty.
func (r *ByNameRef[T]) SetQualifiedName(qn string) {
	r.qname = qn
	r.markDirty()
}

// SetFromDecode populates the value during BSON decode without marking dirty.
func (r *ByNameRef[T]) SetFromDecode(qn string) {
	r.qname = qn
}

// ByNameRefList is a property that holds a list of qualified-name references.
type ByNameRefList[T element.Element] struct {
	propertyBase
	targetType    string
	qnames        []string
	versionMarker int32 // BSON array version prefix: 1 (default) or 3 (for AllowedRoles on pages)
}

// NewByNameRefList creates a ByNameRefList with BSON version marker int32(1).
// Used for AllowedModuleRoles, ModuleRoles, and similar role lists on microflows/nanoflows.
func NewByNameRefList[T element.Element](name, targetType string) *ByNameRefList[T] {
	return &ByNameRefList[T]{propertyBase: propertyBase{name: name}, targetType: targetType, versionMarker: 1}
}

// NewByNameRefListV3 creates a ByNameRefList with BSON version marker int32(3).
// Required for AllowedRoles on Forms$Page (document-level access control).
// Mendix Studio Pro uses version 3 for page AllowedRoles; using version 1 causes
// CE0557 ("At least one allowed role must be selected") even when roles are set.
func NewByNameRefListV3[T element.Element](name, targetType string) *ByNameRefList[T] {
	return &ByNameRefList[T]{propertyBase: propertyBase{name: name}, targetType: targetType, versionMarker: 3}
}

func (r *ByNameRefList[T]) TargetType() string       { return r.targetType }
func (r *ByNameRefList[T]) QualifiedNames() []string { return r.qnames }

// BSONValue returns a versioned BSON array ([]any) with the version marker
// as the first element followed by the qualified-name strings. Mendix requires
// this version prefix for string-only reference lists; omitting it causes
// CE0003 ("entity access is out of date") in Studio Pro.
// Version 1 is used for AllowedModuleRoles on microflows; version 3 for
// AllowedRoles on pages.
func (r *ByNameRefList[T]) BSONValue() any {
	vm := r.versionMarker
	if vm == 0 {
		vm = 1
	}
	out := make([]any, 0, len(r.qnames)+1)
	out = append(out, vm)
	for _, qn := range r.qnames {
		out = append(out, qn)
	}
	return out
}

// Append adds a qualified name to the list and marks the property dirty.
func (r *ByNameRefList[T]) Append(qn string) {
	r.qnames = append(r.qnames, qn)
	r.markDirty()
}

// Remove removes the first occurrence of qn from the list and marks dirty.
func (r *ByNameRefList[T]) Remove(qn string) {
	for i, q := range r.qnames {
		if q == qn {
			r.qnames = append(r.qnames[:i], r.qnames[i+1:]...)
			r.markDirty()
			return
		}
	}
}

// SetQualifiedNames replaces the entire list and marks the property dirty.
func (r *ByNameRefList[T]) SetQualifiedNames(qns []string) {
	r.qnames = qns
	r.markDirty()
}

// SetFromDecode replaces the list during BSON decode without marking dirty.
func (r *ByNameRefList[T]) SetFromDecode(qnames []string) {
	r.qnames = qnames
}

// ByIdRef is a property that references another element by ID.
type ByIdRef[T element.Element] struct {
	propertyBase
	id element.ID
}

func NewByIdRef[T element.Element](name string) *ByIdRef[T] {
	return &ByIdRef[T]{propertyBase: propertyBase{name: name}}
}

func (r *ByIdRef[T]) RefID() element.ID { return r.id }
func (r *ByIdRef[T]) BSONValue() any    { return r.id }

// SetID stores the referenced element's ID and marks the property dirty.
func (r *ByIdRef[T]) SetID(id element.ID) {
	r.id = id
	r.markDirty()
}

// SetFromDecode populates the ID during BSON decode without marking dirty.
func (r *ByIdRef[T]) SetFromDecode(id element.ID) {
	r.id = id
}
