package property

import "github.com/JordtenBulte-OLC/mxcli/modelsdk/element"

// propertyBase holds the common fields shared by all property types:
// name, dirty flag, owner element, and dirty-bit index.
type propertyBase struct {
	name  string
	dirty bool
	owner *element.Base
	bit   uint
}

func (b *propertyBase) Name() string { return b.name }
func (b *propertyBase) Dirty() bool  { return b.dirty }

func (b *propertyBase) Bind(owner *element.Base, bit uint) {
	b.owner = owner
	b.bit = bit
}

func (b *propertyBase) markDirty() {
	b.dirty = true
	if b.owner != nil {
		b.owner.MarkDirty(b.bit)
	}
}
