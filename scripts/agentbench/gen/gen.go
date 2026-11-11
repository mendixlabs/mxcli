// SPDX-License-Identifier: Apache-2.0

// Package gen emits the MDL the agent dev-loop benchmark executes: the
// authoring scripts of its scripted session and the seed of its large fixture.
//
// The output is a pure function of Params — no clock, no randomness, no map
// iteration — so the same parameters produce the same bytes on every machine.
// That is what makes the benchmark's machine-independent counters (unit files
// read, units written) comparable from one night to the next. See
// docs/11-proposals/PROPOSAL_agent_devloop_benchmark.md.
//
// Every statement is `create or modify`, so a script can be executed twice:
// the second run is the benchmark's idempotence step, which must write nothing.
package gen

import (
	"fmt"
	"strings"
)

// Params sizes one generated app.
type Params struct {
	// Prefix names the modules: Prefix01, Prefix02, … (or just Prefix when
	// Modules is 1).
	Prefix string
	// Modules is how many modules to generate.
	Modules int
	// Entities is how many entities each module has. Every entity brings two
	// microflows (validate, save), one overview page and one edit page.
	Entities int
	// Revision changes the body of every microflow without changing its name
	// or signature. Revision 0 and revision 1 of the same Params are the
	// "before" and "after" of a bulk change.
	Revision int
}

// Documents is how many project documents Params generates, the size figure a
// record reports: a domain model per module, and two microflows and two pages
// per entity.
func (p Params) Documents() int {
	return p.Modules * (1 + 4*p.Entities)
}

// ModuleName is the name of module i (0-based).
func (p Params) ModuleName(i int) string {
	if p.Modules == 1 {
		return p.Prefix
	}
	return fmt.Sprintf("%s%02d", p.Prefix, i+1)
}

// EntityName is the name of entity j (0-based) in any module.
func EntityName(j int) string { return fmt.Sprintf("Item%02d", j+1) }

const header = "mdl 1;\n"

// Domain is the script that creates every module, its entities and their
// associations: each entity refers to the one before it, and the first entity
// of each module to the first entity of the previous module, so references
// cross module boundaries at a fixed density.
func Domain(p Params) string {
	var sb strings.Builder
	sb.WriteString(header)
	for i := 0; i < p.Modules; i++ {
		mod := p.ModuleName(i)
		fmt.Fprintf(&sb, "\ncreate module if not exists %s;\n", mod)
		for j := 0; j < p.Entities; j++ {
			ent := EntityName(j)
			fmt.Fprintf(&sb, `
/** %s %d of module %s. */
@position(%d, %d)
create or modify persistent entity %s.%s (
  Name: string(200) not null,
  Description: string(unlimited),
  Amount: integer default 0,
  Price: decimal default 0,
  IsActive: boolean default true,
  DueDate: datetime
);
`, "Benchmark entity", j+1, mod, 100+(j%8)*260, 100+(j/8)*220, mod, ent)
		}
		for j := 1; j < p.Entities; j++ {
			fmt.Fprintf(&sb, "create or modify association %s.%s_%s from %s.%s to %s.%s;\n",
				mod, EntityName(j), EntityName(j-1), mod, EntityName(j), mod, EntityName(j-1))
		}
		if i > 0 {
			prev := p.ModuleName(i - 1)
			fmt.Fprintf(&sb, "create or modify association %s.%s_%s%s from %s.%s to %s.%s;\n",
				mod, EntityName(0), prev, EntityName(0), mod, EntityName(0), prev, EntityName(0))
		}
	}
	return sb.String()
}

// Microflows is the script that creates a validation microflow and a save
// microflow for every entity. The save calls the validation, and the save of
// each entity but the first also calls the save of the entity before it, so
// renaming one entity or re-creating one microflow has callers to resolve.
func Microflows(p Params) string {
	var sb strings.Builder
	sb.WriteString(header)
	for i := 0; i < p.Modules; i++ {
		for j := 0; j < p.Entities; j++ {
			sb.WriteString(Validate(p, i, j))
			sb.WriteString(Save(p, i, j))
		}
	}
	return sb.String()
}

// Validate is the `create or modify` of one entity's validation microflow.
func Validate(p Params, module, entity int) string {
	mod, ent := p.ModuleName(module), EntityName(entity)
	return fmt.Sprintf(`
/**
 * Validates a %[2]s before it is saved (revision %[3]d).
 */
create or modify microflow %[1]s.VAL_%[2]s_Save (
  $Item: %[1]s.%[2]s
)
returns boolean
begin
  declare $IsValid boolean = true;

  if $Item/Name = empty then
    validation feedback $Item/Name message 'Name is required';
    set $IsValid = false;
  end if;

  if $Item/Amount < %[3]d then
    validation feedback $Item/Amount message 'Amount is too low';
    set $IsValid = false;
  end if;

  if $Item/Price < 0 then
    validation feedback $Item/Price message 'Price cannot be negative';
    set $IsValid = false;
  end if;

  return $IsValid;
end;
`, mod, ent, p.Revision)
}

// Save is the `create or modify` of one entity's save microflow.
func Save(p Params, module, entity int) string {
	mod, ent := p.ModuleName(module), EntityName(entity)
	return fmt.Sprintf(`
/**
 * Validates and commits a %[2]s (revision %[3]d).
 */
create or modify microflow %[1]s.ACT_%[2]s_Save (
  $Item: %[1]s.%[2]s
)
returns boolean
begin
  $IsValid = call microflow %[1]s.VAL_%[2]s_Save(Item = $Item);

  if $IsValid then
    change $Item (Description = 'Saved at revision %[3]d');
    commit $Item;
  end if;

  return $IsValid;
end;
`, mod, ent, p.Revision)
}

// Pages is the script that creates an edit page and an overview page for every
// entity. The overview's grid opens the edit page, and the edit page's save
// button calls the entity's save microflow.
func Pages(p Params) string {
	var sb strings.Builder
	sb.WriteString(header)
	for i := 0; i < p.Modules; i++ {
		mod := p.ModuleName(i)
		for j := 0; j < p.Entities; j++ {
			ent := EntityName(j)
			fmt.Fprintf(&sb, `
create or modify page %[1]s.%[2]s_NewEdit folder 'Pages'
(
  params: ( $Item: %[1]s.%[2]s ),
  title: 'Edit %[2]s',
  layout: Atlas_Core.PopupLayout
)
{
  layoutgrid mainGrid {
    row {
      column (desktopwidth: autofill) {
        dataview dataView1 (datasource: $Item) {
          textbox txtName (label: 'Name', attribute: Name)
          textarea txtDescription (label: 'Description', attribute: Description)
          textbox txtAmount (label: 'Amount', attribute: Amount)
          textbox txtPrice (label: 'Price', attribute: Price)
          datepicker dpDueDate (label: 'Due date', attribute: DueDate)

          footer {
            actionbutton btnSave (caption: 'Save', action: call microflow %[1]s.ACT_%[2]s_Save(Item = $Item), buttonstyle: success)
            actionbutton btnCancel (caption: 'Cancel', action: cancel changes)
          }
        }
      }
    }
  }
};

create or modify page %[1]s.%[2]s_Overview folder 'Pages'
(
  title: '%[2]s overview',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid mainGrid {
    row {
      column (desktopwidth: 12) {
        dynamictext heading (content: '%[2]s', rendermode: H2)
        datagrid grid1 (
          datasource: database %[1]s.%[2]s
        ) {
          column (attribute: Name, caption: 'Name') {
            textfilter filterName
          }
          column (attribute: Amount, caption: 'Amount') {
            numberfilter filterAmount
          }
          column (attribute: DueDate, caption: 'Due date') {
            datefilter filterDueDate
          }
          column (caption: 'Actions') {
            actionbutton btnEdit (caption: 'Edit', action: show page %[1]s.%[2]s_NewEdit(Item = $currentObject))
            actionbutton btnDelete (caption: 'Delete', action: delete, buttonstyle: danger)
          }
        }
      }
    }
  }
};
`, mod, ent)
		}
	}
	return sb.String()
}

// SingleEdit is the smallest change an agent makes: one microflow re-created
// with a changed body.
func SingleEdit(p Params) string {
	q := p
	q.Revision = p.Revision + 1
	return header + Save(q, 0, 0)
}

// BulkAlter re-creates every microflow at the next revision: one statement per
// document, each changing it.
func BulkAlter(p Params) string {
	q := p
	q.Revision = p.Revision + 1
	return Microflows(q)
}
