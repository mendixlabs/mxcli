# Change member through an association — check passes it, the writer miscategorizes it, mxbuild rejects it (CE1613)

- **Investigated**: 2026-10-16, against `e0467c692`, mxbuild 11.13.0, blank project
- **Upstream**: none yet — draft
- **Related**: FINDINGS #51 (mdl-executor, the one-qualifier refusal this bug sneaks past), #830 (same CE1613, widget `Attribute` slot)

## Verdict

`change $a (A_B.Title = 'x')` — setting an attribute of the object reached
through association `A_B` — is **not representable** in Mendix's
`Microflows$ChangeActionItem`: the member slot holds either an attribute of the
changed entity itself or an association reference, and no spelling encodes a
path. Yet the statement survives the whole mxcli pipeline:

1. `mxcli check` **skips** dotted members on the assumption exec refuses them
   (`mdl/executor/validate_member_refs.go:432-436`) — the assumption is false
   for the `Assoc.Attr` shape.
2. `exec` writes it as an **association set** with the invalid name `A_B.Title`
   (`mdl/executor/cmd_microflows_builder_actions.go`, `resolveMemberChange` →
   `resolveMemberChangeFallback`, the one-dot branch at `:2416`).
3. `mx check` then fails the project: `CE1613 "The selected association
   'A_B.Title' no longer exists." at Change object activity 'Change 'a'
   (Title)'` — after `mxcli check` and `exec` both reported success.

## Reproduction

```mdl
mdl 1;
create module Hop;
create persistent entity Hop.A (Code: string(50));
create persistent entity Hop.B (Title: string(100));
create association Hop.A_B from Hop.A to Hop.B type reference;

create microflow Hop.CHG_OneHop ($a: Hop.A) returns string as $r
begin
  declare $r string = '';
  change $a (A_B.Title = 'one-hop');
  set $r = 'ok';
  return $r;
end;
```

```bash
mxcli exec repro.mdl -p app.mpr          # reports success, writes the flow
mx check app.mpr                          # CE1613
```

Positive control: `change $a (Code = 'own')` (the entity's own attribute) is
accepted by mxbuild — the defect is specific to the dotted associated-member
form.

## Evidence

**What mxcli wrote** (strings of the stored `.mxunit`):

```
$Type  Microflows$ChangeActionItem
Association  A_B.Title        ← the member landed in the ASSOCIATION slot
Type         Association
Value        'one-hop'
```

**Probe matrix** — the decisive question is whether ANY encoding of a
through-association member exists. Each candidate was written into the stored
BSON by hand (BSON round-trip via `modelsdk/mpr`) and the project checked with
mxbuild 11.13.0:

| Stored member encoding | Result |
|---|---|
| `Association: A_B.Title` (what mxcli writes) | CE1613 "selected association 'A_B.Title' no longer exists" |
| `Attribute: Hop.A_B/Title` | StorageLoadException — `'Hop.A_B/Title' is not a valid AttributeIdentifier` (project **unloadable**) |
| `Attribute: Hop.A_B.Title` | CE1613 "The selected attribute 'Hop.A_B.Title' no longer exists" |
| `Attribute: Hop.B.Title` | CE0226 "Entity 'Hop.A' does not contain attribute 'Hop.B.Title'" |
| `Attribute: Hop.A.A_B.Title` | StorageLoadException — invalid AttributeIdentifier (4-part identifiers do not parse) |
| `Attribute: Hop.A.Code` (own attribute, control) | clean |

The identifier grammar admits only `Module.Entity.Attribute` **of the changed
entity** (CE0226 proves mxbuild checks exactly that). There is no slot for the
association step. Studio Pro's idiom is `retrieve $b from $a/Hop.A_B` then
`change $b (Title = …)`.

## Root cause, layer by layer

**Check** — `mdl/executor/validate_member_refs.go:432-436` (`checkMembers`):

```go
// A qualified member is a different question with a different answer,
// and exec already refuses the one-qualifier form that cannot be an
// attribute (FINDINGS #51). Left alone here rather than second-guessed.
if strings.Contains(name, ".") {
    continue
}
```

The trust is misplaced. FINDINGS #51's refusal fires only when the pre-dot
segment **is a module** whose domain model lacks the association
(`resolveMemberChange`, the `strings.Count(memberName, ".") == 1` branch at
`cmd_microflows_builder_actions.go:2275`). For `A_B.Title` the pre-dot segment
is an **association name, not a module**, so `GetModuleByName("A_B")` returns
nil, the whole guarded block is skipped, and the member falls through to
`resolveMemberChangeFallback` — whose one-dot heuristic
(`:2416`, "1 dot => association qualified by module") writes
`AssociationQualifiedName = "A_B.Title"` unchecked.

**Writer** — `resolveMemberChangeFallback`'s one-dot branch has no
existence validation when reached via the module-not-found path. It is the
only leak: when the pre-dot segment *is* a real module, #51's refusal fires;
when it is not, the invalid name is written verbatim.

**Platform** — `Microflows$ChangeActionItem` carries `Attribute`
(`Module.Entity.Attribute` of the changed entity) and `Association` (set the
reference itself). Nothing else. Through-association member changes are not a
Mendix feature of this activity.

## Fix direction

Both layers must move, and the writer is the one that must refuse:

1. **Writer** (`resolveMemberChangeFallback` one-dot branch, and/or
   `resolveMemberChange` before the fallback): when the pre-dot segment is not
   a module of the project, refuse with the working idiom —
   `a member reached through an association is not a change item: retrieve through the association and change the retrieved object`
   plus the exact association to retrieve through. This is the same shape as
   #51's refusal, one branch earlier.
2. **Check** (`checkMembers`): stop blanket-skipping dotted members. A member
   with exactly one dot whose pre-dot segment is not a module is the same
   unrepresentable form and should be reported at check time, so `mxcli check`
   and `mx check` agree again. (Two dots — `Module.Entity.Attribute` — stays
   legal and keeps the skip.)

`mxcli check` refusing while exec also refuses is the target state; leaving
check silent makes the gap invisible until a whole build later, which is the
failure mode #562 documented for member drops.

## Repro assets

`/tmp/opencode/hopverify/` (session-local, not committed): `script1.mdl`
(entities + the failing flow + multi-hop expression/sort flows that all pass
mxbuild), `script3.mdl` (own-attribute positive control), `hopverify.mpr.bak`.
The probe tooling was a throwaway `bson.Marshal` round-trip over
`Reader.GetRawUnitByName` + `WriteTransaction.WriteUnit`; it is not preserved.
