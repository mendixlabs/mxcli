---
title: The Keyword Set Leaks Into User Data
category: bug-pattern
last-synced: a20932c1
covers:
  - mdl/grammar
sources:
  - .claude/skills/fix-issue/findings/mdl-grammar/
  - mdl/grammar/MDLLexer.g4
  - mdl/grammar/MDLParser.g4
  - mdl/grammar/MDLSecurity.g4
  - mdl/executor/identifier_quoting.go
---

> **Do not duplicate**: the quoting rules a user needs are in the skills and
> CLAUDE.md ("Quoting Escapes Parser Keywords, Not Platform-Reserved Member
> Names"); each collision's fix is in the findings. This page is about the shape.

## What this is

MDL has a large keyword vocabulary, and those keywords occupy positions where
user data also lives — a widget called `List`, an attribute called `Template`, an
XPath predicate calling `trim()`, a negative number, a string containing a
newline. Eight `mdl/grammar` findings are a collision between the two.

The distinguishing question is not whether a collision happens but **how it
fails**. A parse error is recoverable: the user sees it and quotes the name. The
same collision resolving to a *different valid parse* is not, and several of
these did exactly that.

## How it fits

**Silent is the dangerous half.** A widget conditional using a function whose
name is also a lexer keyword did not error — it was dropped, so the widget lost
its visibility rule and the page built cleanly. A describer emitting a
keyword-named widget bare produced output that failed only when someone re-ran
it. The loud failures in this class cost minutes; the quiet ones cost a
round trip.

**A grammar fix alone can be worse than the bug.** Accepting an unquoted negative
number in XPath made the parser succeed while the AST builder had no case for the
new alternative — so the constraint serialized as `[Amount > ]`, a dropped
operand instead of a loud error. **A grammar alternative and its visitor case are
one change**, and only a round-trip helper caught it; the write path used the raw
text and looked fine.

**Prove a relaxation with a control binary, not by reading.** Making a keyword
optional or a rule more permissive can change how unrelated input parses. The
method that works: stash the `.g4`, regenerate, build a control binary, and sweep
every example with both. Thirteen scripts failed with the change — the *same*
thirteen, all pre-existing.

**Derive the accepted set from the grammar, in a test.** Where a keyword list is
maintained on one side and derived on the other, the next widget package
re-opens the gap. A test that reads the accepted keywords straight out of the
`.g4` cannot drift from it, and can carry the fix instructions in its failure
message.

**A renderer that stringifies an enum outgrows its grammar silently.** An emitter
that formats a value by name will happily print the ninth member of an
enumeration that the grammar knows eight of. Switching on concrete types cannot
do that, which is why the sibling emitter never had the bug — and where
stringifying is unavoidable, the guard is a describe-then-parse loop over the
enumeration's full membership.

**Every bare `IDENTIFIER` in a name position is a time bomb that the next lexer
keyword arms.** The collision does not need a new user name — it needs a new
*keyword*. Promoting `ai` and `sample` to lexer tokens broke
`alter enumeration … drop value Sample;` (create already used a keyword-tolerant
name rule, alter did not); `Region` has lexed as `SCROLLREGION` for as long as
scroll regions have existed, so `grant read on M.E (Region)` was a parse error
while `create entity` accepted the same name unquoted. Two rules fall out, and
both are greps rather than judgement:

- **When a PR adds a lexer token, grep the grammar for bare `IDENTIFIER` in name
  positions** — anything not `identifierOrKeyword` or a `*Name` rule — and try
  the new word there. The round-trip corpus only catches it if some app happens
  to use the word.
- **A name a statement can declare must be usable everywhere it is referenced.**
  The asymmetry is the bug: one rule out of the whole security grammar lacked
  `| keyword`, and a user who could create the attribute could not grant on it.

The same applies to any new `( key: value )` option list, where the keys are
usually English words the lexer already owns: keying one on `IDENTIFIER` made the
feature's **only** option a parse error on the day it shipped.

**A catch-all alternative is a silent no-op for everything its visitor does not
list.** `helpStatement: IDENTIFIER (word …)*` matched any statement beginning
with an identifier, and the visitor filtered by `switch word { case "help": … }`
— so `craete module Foo;` parsed cleanly, did nothing, and exited 0. A misspelt
keyword is not a typo the user gets told about; it is a statement that vanishes.
Move the filter **into** the grammar as a predicate so the parser refuses what
the visitor would ignore. The method for finding what an old catch-all used to
swallow is worth keeping: do not diff the grammars (the word is in neither) —
dump the old binary's parse tree for the failing line, which named
`(helpStatement private)` at once, then sweep a corpus for every error-free old
parse that reached that rule.

**An AST field cannot be singular where the grammar allows repetition.** Two
predicates on one XPath step parsed and the second was dropped, because the step
carried one `Predicate` field — and the stored-constraint reader did not report
it either, since ANTLR's recovery consumed the stray bracket up to EOF and the
EOF check passed. When relaxing a rule to accept repetition, the AST, every
serializer and every rewriter change with it.

**A multi-word token collides in pairs, which no keyword check sees.** The
`error message` clause is one lexer token, so an attribute named `Error`
followed by the `message` keyword (`validation feedback $E/Error message 'x'`,
verbatim describe output) lexed as the clause. Neither word is a problem alone,
so the lexer-driven quoting in describe passes `Error` unquoted. Fix it where the
token is made: a lexer predicate on context (here "not after a `/` or `.`
separator") covers every rule at once, but walking backwards over line breaks
lets a comment ending in `.` or `/` pass for a separator. Put the comment cases in
as controls.

**Quoting is not one question.** Quoting escapes MDL's *parser* keywords. It does
not escape Mendix's platform-reserved member names, and it does not help in an
OQL alias. Three vocabularies stack, and a rule that conflates them produces
confident, wrong advice.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  collisions and their controls
- [[describe-round-trip-gaps]] — where an unquoted emit surfaces
- [[platform-semantics-gaps]] — the other two vocabularies quoting does not reach
- [[capability-gap-as-parse-error]] — when the parse error is honest and the
  capability is what is missing
