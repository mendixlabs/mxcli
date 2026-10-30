# Working Outside Studio Pro

A project edited with mxcli, git and an editor is sometimes opened in Studio Pro
afterwards. Several kinds of state that Studio Pro depends on are invisible to
`mx check`, and mxcli checks them.

## The ContentsHash index: `mxcli fix hashes`

In an MPR v2 project (Mendix 10.18 and later) the `.mpr` file indexes every
`mprcontents/**/*.mxunit` file by its hash: `Unit.ContentsHash` is
`base64(SHA-256(file))`. mxcli's writer and Studio Pro keep the two in step.
Anything else that changes a `.mxunit` does not: restoring a unit with
`git checkout`, `git restore`, a revert or a merge leaves the index describing
bytes that are no longer on disk. `mx check` and MxBuild read the files and do
not notice; Studio Pro uses the index to decide what changed.

```bash
mxcli fix hashes -p app.mpr            # verify; exits 1 when something is off
mxcli fix hashes -p app.mpr --repair   # rewrite mismatched hashes from the files
```

| Reported | Meaning | `--repair` |
|----------|---------|------------|
| `MISMATCH` | the file's hash differs from the stored one | rewrites the stored hash from the file |
| `MISSING` | the `.mpr` indexes a unit whose file does not exist | reported only — restore the file |
| `ORPHAN` | a `.mxunit` file no unit in the `.mpr` indexes | reported only — `mxcli diag --check-units --fix` removes it |

The repair treats the files as the source of truth and never changes them. It
updates every hash in one SQLite transaction, and, like every mxcli write, is
refused while Studio Pro has the project open (the `.mpr.lock` beside it).

An MPR v1 project (below Mendix 10.18) keeps each unit's contents inside the
`.mpr`, in the same row as its `ContentsHash`, and Studio Pro fills the hash
there too, with the same formula over the stored contents. No file can drift
from it, but mxcli before this release replaced a unit's contents and left the
old hash in place, or stored none for a new unit. On v1 the command compares
each hash with the contents beside it, reports `MISMATCH` only, and `--repair`
rewrites the hash from the contents.

`mxcli exec` and `mxcli docker check` run the verify too and print a one-line
warning when it finds a mismatch or a missing file. It never fails them;
measured on a 900-unit project, the whole verify takes about 0.2 s. They skip
MPR v1, where the verify reads every unit's contents.

## Property names Studio Pro's merge compares: `mxcli fix property-sets`

Mendix's diff and merge engine compares two revisions of an element by its
property **names** before any value, and throws when they differ:

```
Objects with ID … of type Microflows$RestOperationCallAction do not have the same properties.
```

The model is valid — `mx check`, the build and the runtime fill an absent
property with its default — but `mx diff`, merging and Studio Pro's Version
Control panel fail on it once Studio Pro has saved the document with its full
property set. Every mxcli write gives a unit the property set Studio Pro writes
for the project's Mendix version. A unit written by an older mxcli and not
rewritten since keeps its gaps; this command reaches those.

```bash
mxcli fix property-sets -p app.mpr --dry-run   # list the units that would change; exits 1 when one would
mxcli fix property-sets -p app.mpr             # write them
```

Each unit's report counts the keys added (with the value Mendix gives an absent
one) and the empty keys stripped (properties the project's version does not
declare yet). A project user role with no `GUID` — older mxcli wrote them
without one — gets its own `$ID` as its GUID, the value a role mxcli creates
now gets; every mxcli write of the security settings does the same. Without
it the build invents a random GUID each time, so the runtime can only match
the role's database row by name. A role that already has a GUID keeps it.
Nothing else changes: no element `$ID` moves, and no existing `GUID` is
touched. A second run writes nothing.

The filled GUID protects a later **rename** of the role only in a database
first synchronised after the fill. A database synchronised before it keeps
the old random GUID and still finds the role by name, so renaming the role
there still deletes its row and the user assignments that are not demo users. A unit holding a *value* in a
key its version does not declare is reported and left alone, and, like every
mxcli write, the command is refused while Studio Pro has the project open.

The values the command adds were measured with Mendix's own tools on specific
Mendix versions. A key that no measurement covers on your version is not
filled, but the command still finds it. Studio Pro writes the same property
names on every element of a type, so the command also lists each name that
some elements of a type carry and others lack:

```
  MIXED     Microflows$Nanoflow.ExportLevel: on 14 of 15 element(s); without it: 71e6faf6-… [Microflows$Nanoflow ACT_Login]
```

It leaves those names as they are and exits 1. Report them with your Mendix
version so the value can be measured.

### The repair reaches forward, not back

A merge compares each document both branches changed with its revision in the
merge base, the commit the two branches share. The repair completes the
property names from its own commit on, and so does any Studio Pro save of the
document; the revisions before it in history keep their gaps. A merge or rebase
whose base is one of those revisions still fails on every such document,
whichever branch carries the full set. Measured with `mx merge`, Studio Pro's
merge engine, on 11.13.0, for an access rule an mxcli before 0.26 created: the
exception when the other branch had been saved by Studio Pro, the same exception
when it had been repaired by this command, and a clean merge once the repaired
revision is the base.

So run the repair once, on the main branch, in a commit of its own, when few
branches are open, and start new branches from it. An older branch still merges
cleanly unless it changed a document that was incomplete in its merge base;
that branch needs the steps below.

### Getting past a merge that fails with "do not have the same properties"

Studio Pro looks for an unfinished merge or rebase when it opens a project and
runs it again, so a project whose merge fails does not open until the merge is
aborted. The way through is to make your branch stop changing the documents the
merge cannot compare, merge, and redo your edits to them afterwards.

1. **Abort** from the command line: `git merge --abort`, or `git rebase --abort`.
2. **Find the documents.** The exception names only the first element it meets.
   The merge cannot compare a document that an older mxcli left incomplete in
   the merge base *and* that both branches changed since:

   ```bash
   upstream=origin/main                     # the branch you are merging
   base=$(git merge-base HEAD $upstream)
   git worktree add ../merge-base $base
   mxcli fix property-sets -p ../merge-base/app.mpr --dry-run   # NEEDS <unit id> [type]
   git worktree remove ../merge-base
   comm -12 <(git diff --name-only $base HEAD -- mprcontents | sort) \
            <(git diff --name-only $base $upstream -- mprcontents | sort)
   ```

   The units in both lists are the ones to take back. A unit's file is
   `mprcontents/<first two>/<next two>/<unit id>.mxunit`.
3. **Take your edits back.** Note what you changed in those documents
   (`mxcli diff-local -p app.mpr --ref $base..HEAD` prints it as MDL), then put
   each one back to its merge-base version, re-index it, and commit:

   ```bash
   git checkout $base -- mprcontents/01/31/0131b342-dc00-4619-9e90-64c9de7e2a50.mxunit
   mxcli fix hashes -p app.mpr --repair     # the .mpr indexes every unit by its hash
   git commit -am "Take back edits the merge cannot compare"
   ```

4. **Merge** again in Studio Pro. Your branch no longer changes those
   documents, so the merge takes the other branch's version without comparing
   them. Merge rather than rebase: a rebase replays your commits one at a time,
   and the first one that changed such a document fails again.
5. **Redo the edits** in Studio Pro, or by running the MDL from step 3, and
   commit. That revision carries the full property set, so a merge based on it
   or on anything later is unaffected.

Never get past the error by deleting and re-creating an entity, attribute or
association: the new element gets a new GUID, and on the next deployment the
runtime drops the old one's table or column, with its data.

Measured with `mx merge` on 11.13.0 (`testdata/pedapp`, an access rule an mxcli
before 0.26 created, and both branches editing the same domain model): the merge
fails with the exception before step 3 and succeeds after it, the merged project
keeps both branches' other changes, and `mx check` reports exactly what it
reports on the untouched project.

## Git states that crash Studio Pro

Studio Pro 11.13 fails to open a project with *"Unable to find 'system'
property in 'system'"* when the project folder is a git repository and

- the checked-out branch has **no upstream** — remedy:
  `git push -u origin <branch>` before opening the project (or
  `git remote add origin <url>` first, if the repository has no remote); or
- git reports **"detected dubious ownership"** — typical for a folder shared
  between the host and a devcontainer, owned by a different user on each side.
  Remedy, on the machine that runs Studio Pro:
  `git config --global --add safe.directory <path>`.

`mxcli docker check`, `mxcli run --local` and `mxcli diag -p app.mpr` warn about
both. The warnings are advice and never fail a command. They are silent:

- outside a git repository, or when git is not installed;
- on a **detached HEAD** — how CI systems and `git checkout <sha>` leave a
  repository; there is no branch to push, and nobody opens such a checkout in
  Studio Pro;
- when the `CI` environment variable is set, or `MXCLI_NO_GIT_WARNINGS=1`.

Dubious ownership is judged by the git mxcli runs. Inside a devcontainer that is
the container's git, which may see ownership differently from the host's Studio
Pro — so a clean report from inside the container does not rule it out on the
host.

## One report: `mxcli diag -p`

```bash
mxcli diag -p app.mpr
```

prints mxcli's diagnostics followed by a *Project* section with both checks.

## One commit, all the steps: `mxcli git commit`

Studio Pro's *Commit* does more than `git commit`: it keeps the ContentsHash
index in step, stages exactly the files a Mendix repository should ship, and
attaches the mx_metadata note. `mxcli git commit` is that sequence for commits
made outside Studio Pro:

```bash
mxcli git commit -m "Add order history page" -p app.mpr
mxcli git commit -m "Fix stock filter" -p app.mpr --push
mxcli git commit -m "WIP" -p app.mpr --dry-run
```

In order it:

1. repairs ContentsHash mismatches from the files on disk (`MISSING` and
   `ORPHAN` entries have no automatic repair and refuse the commit);
2. stages everything, then unstages the files Studio Pro never commits:
   `project-settings.user.json`, `*.mpr.lock`, `*.mpr.bak`, and the project's
   own `.mendix-cache/`, `deployment/`, `theme-cache/` and `.mxcli/`, the
   folders Studio Pro, mxbuild and mxcli create beside the `.mpr`. A folder of
   the same name further down, such as `resources/deployment/`, is committed;
3. runs `git commit` with your message and your git identity;
4. computes the mx_metadata note from the new commit and attaches it, and
   prints the `git push origin <branch> refs/notes/mx_metadata` command —
   `--push` runs it for you, because the branch and the notes ref must travel
   together.

The commit is refused while Studio Pro has the project open (the `.mpr.lock`
beside it), and during an unresolved merge — `git add -A` would otherwise
commit the conflict markers. `--dry-run` verifies and reports (including
ContentsHash drift, which it does not repair — a dry run writes nothing),
and reports what it would commit without committing. It stages into a copy of
the index, so whatever you had staged is left exactly as it was.

## Merging branches: `mxcli git merge`

`git merge` cannot merge Mendix models: the `.mpr` is SQLite and every
`.mxunit` is BSON, so two sides changing the model always meet a binary
`CONFLICT (content)` — there is nothing for git's textual merge to work on.
Studio Pro's *Merge Changes Here* resolves it with a model-aware merge, and
that engine ships as a command: `mx merge BASE MINE THEIRS` over three
project folders (it runs on Linux devcontainers too; the older `mx git-merge`
file form is Windows-bound).

`mxcli git merge <ref>` puts the two together:

```bash
mxcli git merge feature/orders -p app.mpr
mxcli git merge origin/main -p app.mpr --push
```

`git merge <ref>` runs first; a merge git resolves itself (disjoint files,
a fast-forward) is annotated with the mx_metadata note like `mxcli git
commit` does. On a model-file conflict the base and theirs revisions are
checked out into temporary worktrees and the three project folders go to
`mx merge` (resolved for the project's Mendix version); the engine writes
its result into the working tree and owns the `.mpr`:

- what the engine auto-resolves is staged, hash-checked and committed as the
  normal two-parent merge commit, with the note attached. The engine's
  `.mpr` carries both sides' document sets, so a unit one side added while
  the other changed existing ones is kept, not dropped;
- a conflict the engine reports as `ManualResolutionRequired` (exit 2) is
  never merged silently: the merge is aborted, and **both sides' changes are
  printed for review** — at property level, from `mx diff` ("New type >
  Length: 100 → 150" on ours, "→ 200" on theirs). Then decide:
  `--plan <file>` resolves **per unit** from the plan template written beside
  the .mpr (`<mpr>.merge-plan.json`, which `mxcli git commit` never stages):
  set `"take"` to `ours`/`theirs` per unit, and the `.mpr` index is repaired
  from the mixed files (modified-on-both units only; added/deleted/moved
  conflicts go to Studio Pro). `--ours` or `--theirs` takes one side of every
  conflicted unit, hash-gated: when the sides' document sets differ (one side
  added or deleted a unit), committing a whole side would orphan the other
  side's documents, so the commit is refused and the merge aborted. Any other
  engine exit is mx failing, not a decision: it is reported and the merge is
  aborted cleanly. Resolutions that must mix both sides *inside* one unit
  stay in Studio Pro (*Version Control › Merge Changes Here*);
- a conflict in non-model files (text sources) is left in place for manual
  resolution, like plain git: resolve them, `git add` them, and re-run the
  command — it finishes the merge (model conflicts included) and commits.

Merging is still never done by plain `git merge` on a Mendix repository: use
`mxcli git merge` or Studio Pro, never a textual merge of `mprcontents/`.
Like every mxcli write, the command is refused while Studio Pro has the
project open.

## Auditing a repository: `mxcli git doctor`

A repository that has known only plain git misses several things Studio Pro
wrote when it committed, and they are invisible one by one. `mxcli git doctor`
reports all of them in one read-only pass; `--fix` repairs:

```bash
mxcli git doctor                       # report only; exits 1 while an issue remains
mxcli git doctor -p app.mpr --fix      # repair, computing real notes from the model
mxcli git doctor --fix --version 10.21.0
```

| Check | `--fix` |
|-------|---------|
| local git config: `core.autocrlf=False` (a checkout must not rewrite line endings inside `.mxunit` files; any other value is rewritten), `mendix.commits-since-gc`, `mendix.lineEndingResetDone` — the `mendix.*` values are Studio Pro's own bookkeeping and are only backfilled when missing | sets/backs them |
| the remote URL as configured: Studio Pro can only use HTTPS | rewrites SSH (`git@host:org/app.git`, `ssh://…`) to HTTPS; other non-HTTPS URLs are reported unchanged. A `url.<base>.insteadOf` rule in your git config that makes git use another URL is reported, and left alone |
| the remote's `refs/notes/mx_metadata` | reported only — push the notes ref together with the branch |
| the local `refs/notes/mx_metadata` holds the remote's notes: `git clone` and a plain `git fetch` never fetch them, so a fresh clone or a CI checkout has none | fetches them, as a fast-forward only; refused when the local ref has notes the remote does not |
| each commit on the checked-out branch and its mx_metadata note: missing notes make Studio Pro show `(unknown)`; a malformed note it cannot read at all | computes real notes from the model (`-p`), falling back to a template note with a `--version` ModelerVersion where the model at a commit cannot be read (MPR v1 history, a revision without the `.mpr`); when no version is known the newest valid note in history supplies it |

Notes Studio Pro wrote are never touched, with or without `--fix`. `--fix`
does not push.

The commits are judged only once the local notes ref holds the remote's. Until
then the report names the unfetched ref and the command that fetches it, rather
than calling every commit the remote annotates unannotated. In CI, fetch the
notes before the audit:

```bash
git fetch origin refs/notes/mx_metadata:refs/notes/mx_metadata
mxcli git doctor -p app.mpr
```

## Commit metadata: `mxcli git note`

Studio Pro attaches a git note under `refs/notes/mx_metadata` to every commit
it makes:

```json
{"BranchName":"feature-x","ModelerVersion":"11.13.0",
 "ModelChanges":[{"Status":"Modified","UnitID":"…","UnitType":"Forms$Page",
                  "UnitName":"Home_Web","Module":"MyModule"}],
 "RelatedStories":[],"SolutionVersion":"","MPRFormatVersion":"Version2","HasModelerVersion":true}
```

It reads the note to check that a revision's Mendix version is compatible and to
show what the revision changed. A commit made with plain git has none, and
Studio Pro 11.13 back-fills one with `"(unknown)"` values and no changes.

```bash
mxcli git note -p app.mpr            # preview the notes for commits not yet pushed
mxcli git note -p app.mpr --write    # attach them
git push origin <branch> refs/notes/mx_metadata
```

The note is computed from the commit: the `.mpr` unit index and the
`mprcontents` units at the commit and at its parent. With no commit argument it
covers `@{upstream}..HEAD`, or `HEAD` when the branch has no upstream. A note
Studio Pro wrote is kept unless you pass `--force`; a placeholder is replaced.

| Status | Meaning |
|--------|---------|
| `Added` / `Deleted` | the unit is new / gone. A deleted module is listed once, as its `Projects$ModuleImpl` |
| `Modified` | the unit's content changed. A re-serialisation that only mints new element IDs is not a change |
| `Moved` | the unit moved to another folder (its container in the `.mpr` changed) |

Measured against Studio Pro's own notes on a Team Server repository: for
ordinary commits the note is identical, header and change list. For a Mendix
**version upgrade** the header is identical but the change list is longer than
Studio Pro's, because an upgrade re-serialises units in ways only Studio Pro
knows to ignore. Upgrades are done in Studio Pro, which writes its own note. MPR
v1 projects are not supported.

**Push the notes ref soon.** `git push` does not push notes, and while the
project is open Studio Pro fetches every few minutes and force-replaces the local
`refs/notes/mx_metadata` with the Team Server's copy, discarding notes that were
not pushed. If that happens, `mxcli git note --write` again: the discarded notes
have become placeholders, which it replaces.

The whole workflow (server branches, what to stage, never merging model history
with git) is in the `teamserver-git` skill that `mxcli init` installs.
