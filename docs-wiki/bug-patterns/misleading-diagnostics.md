---
title: A Wrong Hint Is Worse Than No Hint
category: bug-pattern
last-synced: a918689d
covers:
  - mdl/visitor
sources:
  - .claude/skills/fix-issue/findings/mdl-visitor/
  - mdl/visitor/visitor.go
---

> **Do not duplicate**: the individual hints and their trigger conditions live in
> the findings and in `mdl/visitor/`. This page is about why the class is graded
> higher than "the message could be clearer".

## What this is

ANTLR's raw errors name tokens, not intentions: `no viable alternative at input
'add<Name>'` for a missing keyword, `mismatched input '=' expecting ')'` for a
caption written with the wrong separator. mxcli adds hints on top. Six
`mdl/visitor` findings are those hints firing on the wrong thing and **sending
the reader somewhere the problem is not**.

A missing hint costs the reader a minute of confusion. A wrong one costs however
long they spend acting on it — and in the reported cases they blamed their
quoting, renamed an attribute that was fine, or concluded a construct was
unsupported.

## How it fits

**A pattern broad enough to be useful is broad enough to be wrong.** The
"unescaped apostrophe" hint matched any short lowercase word, so every genuine
parse error on `on`, `in`, `as`, `to` or `by` was diagnosed as a quoting mistake.
Narrowing it to the actual contraction suffixes — `s`, `t`, `d`, `m`, `re`, `ve`,
`ll` — keeps the real cases and drops the false ones. A hint's precision matters
more than its coverage, because its whole value is that the reader trusts it.

**Key the hint off the source line, not the parser message.** The token error for
a misplaced `index` clause lands on the index *name*, so the message never
mentions `index` at all and no message-matching hint can fire. Reading the
offending line is what makes the diagnosis possible — and it also lets the hint
discriminate against the valid form of the same construct.

**A hint that recommends a rename is a strong claim.** One offered three
alternative names for an attribute that parsed perfectly well elsewhere, because
it conflated two kinds of reserved word: an MDL *parser* keyword, which quoting
escapes and the model keeps, and a name the *platform* reserves, which quoting
does not help with at all. Conflating them is expensive in both directions —
advising a quote where the platform reserves the name produces something that
parses and then fails the build.

**Some errors are the platform's, and the hint should say so.** Mendix XPath
cannot compute values, so `[Seq = $Game/MoveSeq + 1]` is a limitation rather than
a syntax mistake; a bare `mismatched input '+'` reads as the latter. Naming the
constraint turns a dead end into a redesign.

**A bare parse error reads as a missing feature, and gets filed as one.**
`Height: n` on a domain-model annotation produced `mismatched input 'Height'`,
which was reported as mxcli failing to expose a property — and Mendix stores no
annotation height at all: the note auto-sizes to its caption, and there is
nowhere to write one. The honest fix is a hint naming the properties that *do*
exist, which closes the report instead of opening a feature. Settling "does this
property exist" needed four sources and **the generated metamodel was not
enough**: it is a snapshot of one Mendix version, so a property added later is
invisible to it. Mendix's own serializer (`mx dump-mpr`) and, decisively,
`mx convert` — which rewrites the model to a new version and therefore has to
know every key — are the cheap oracles.

**When the grammar is right and the mistake is predictable from the line, the fix
is a hint, not a grammar change.** An Atlas icon name with a hyphen
(`Atlas_Core.Atlas.add-circle`) cannot be a qualified name, and quoting the whole
thing would have required accepting a string where a reference belongs —
breaking the rule that references are qualified names and needing its own
resolution path. The grammar stayed; a hint keyed on the source line names the
working spelling. This is the boundary with a capability gap: the question is not
"is the input reasonable" but "is there a spelling that works".

**A hint must stand down on a line that already failed.** Error recovery truncates
a statement wherever it resynchronised, so a check that reads a statement's last
tokens can report a missing terminator that is plainly present in the source —
a second, phantom mistake stacked on the real one. See
[[the-visitor-walks-a-failed-parse]].

**A hint cannot repair a cascade that recovery built.** A microflow header
without its parameter parentheses got the right hint on ANTLR's first error, and
two more errors underneath it — one three lines into the body, reported after
recovery had already closed the statement. Once the parser has left the
statement, nothing in the error listener can tie the later error back to the
mistake, because the recovered parse no longer knows where the statement ends.
The layer that can is the grammar: accept the wrong form only so the visitor
refuses it, and the cascade never starts — one error, at the mistake, quoting
the author's own header back corrected. Dump the rule-context stack at each
error first; a stack of just the program rule is the sign.

**A lint rule with several causes must not name only one.** The same failure
shows outside the parser: MPR008 told every overlapping pair that "each
statement needs its own @position", while an overlap involving a loop is a
spacing problem — the loop's box grows with its body — so authors added
annotations that were already there. The rule already measured each box by its
own stored size; asking *which kind* of box overlapped was enough to name the
loop, its size and the gap it needs.

**The reachable-form question.** Several of these are not really about wording:
the reader's next question is "then how *do* I write it?", and a hint that names
the working spelling answers it. Where there is no working spelling, saying so is
still better than a token error — that is the boundary with
[[capability-gap-as-parse-error]].

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the hints,
  their triggers and their controls
- [[capability-gap-as-parse-error]] — when the honest hint is "you cannot"
- [[keyword-collisions]] — the two kinds of reserved word, kept apart
- [[the-visitor-walks-a-failed-parse]] — where recovery produces a crash rather
  than a wrong message
- [[visitor-wiring-gaps]] — where a runtime message contradicting the script is
  the tell for a shape-disambiguated rule
