// SPDX-License-Identifier: Apache-2.0

// cmd_brain.go - `mxcli brain` : project knowledge mxcli cannot compute
package main

import (
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var brainCmd = &cobra.Command{
	Use:   "brain",
	Short: "Project knowledge mxcli cannot compute (docs/brain/)",
	Long: `Record and check the project knowledge mxcli cannot compute.

Two halves. DECISIONS: why a pattern was chosen here, which marketplace version
broke what, what a recurring mxbuild error means in this app. THE PLAN: the
requirements being built from, grouped into slices, when the source is a
specification, a prototype or a conversation rather than issues in a tracker.

Anything mxcli CAN answer does not belong here — entities, microflows, pages,
bindings and references are all queryable, and a note that transcribes them is a
note that will disagree with the project.

Entries live in docs/brain/, committed and reviewed like any other change. A
decision's anchors decide its file: @Sales.Order puts it in modules/Sales.md, and
one with no anchor is cross-cutting and lives in project.md. A requirement goes
to its slice, plan/<slice>.md. That is what lets a session load the shards for
the modules it is touching instead of the whole store.

The two kinds differ in which way their anchors point, and it decides everything
else. A decision's anchor points BACKWARD at what exists, so one that stops
resolving means the decision is stale and 'check' fails. A requirement's points
FORWARD at what is intended, so one that does not resolve means not built yet —
which is why 'brain plan' can report progress derived from the model instead of
from a status column that goes stale the moment someone builds something.

An agent captures; a person promotes. The queue is in .mxcli/ and is git-ignored,
so nothing reaches a pull request until someone has looked at it.

The brain can exist before the project. Name its directory with -C <dir>, or with
-p <app.mpr> for the directory an .mpr is (or will be) in; neither needs the
project to exist. 'mxcli new' into that directory adopts the brain, and from then
on 'plan' and 'check' with the same -C count against the model.`,
	Example: `  mxcli brain init -p app.mpr
  mxcli brain init -C ./workspace          # before the project exists
  mxcli brain capture "Orders are committed by Finance, not Sales" -a @Sales.Order -a @Finance.ACT_Post -p app.mpr
  mxcli brain staged -p app.mpr
  mxcli brain promote a1b2c3 -p app.mpr
  mxcli brain capture "Orders must be approvable by a manager" --slice 02-approvals -a @Sales.ACT_Order_Approve -p app.mpr
  mxcli brain brief --slice 02-approvals -p app.mpr
  mxcli brain plan -p app.mpr
  mxcli brain check -p app.mpr
  mxcli brain show -p app.mpr`,
}

var brainInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create docs/brain/ (refuses a docs/brain/ it did not write)",
	// No arguments: the directory is -C. Accepting one and ignoring it wrote
	// the store wherever -p pointed instead of where the caller asked.
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		dir := brainProjectDir(cmd)
		written, err := brain.NewStore(dir).Init()
		if err != nil {
			brainFatal(err)
		}
		if len(written) == 0 {
			fmt.Printf("Already initialised: %s\n", filepath.Join(dir, brain.StoreDir))
			return
		}
		for _, p := range written {
			fmt.Printf("Created %s\n", p)
		}
		fmt.Println("\nCapture something with:  mxcli brain capture \"<text>\" -a @Module.Element")
	},
}

var brainCaptureCmd = &cobra.Command{
	Use:   "capture <text>",
	Short: "Queue something worth remembering (does not commit it)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := brainProjectDir(cmd)
		anchors, _ := cmd.Flags().GetStringSlice("anchor")
		slice, _ := cmd.Flags().GetString("slice")
		// --slice is the only signal needed: a requirement is a requirement
		// because it belongs to a deliverable slice, so there is no second
		// --kind flag to contradict it.
		var (
			e   brain.Entry
			err error
		)
		open, _ := cmd.Flags().GetBool("open")
		switch {
		case open:
			e, err = brain.NewQuestion(args[0], anchors, slice, time.Now())
		case slice != "":
			e, err = brain.NewRequirement(args[0], anchors, slice, time.Now())
		default:
			e, err = brain.NewEntry(args[0], anchors, time.Now())
		}
		if err != nil {
			brainFatal(err)
		}
		added, err := brain.NewQueue(dir).Append(e)
		if err != nil {
			brainFatal(err)
		}
		if !added {
			fmt.Printf("Already queued as %s — not added again.\n", e.ID)
			return
		}
		what := "Queued"
		if e.Open {
			what = "Queued open question"
		}
		fmt.Printf("%s %s -> would promote into %s\n", what, e.ID, shardLabel(e.Shard()))
		fmt.Println("Review with 'mxcli brain staged'; commit it with 'mxcli brain promote " + e.ID + "'.")
	},
}

var brainStagedCmd = &cobra.Command{
	Use:   "staged",
	Short: "List the queue, with the shard each entry would land in",
	Long: `List what has been captured and not yet promoted.

With no flags this is the review list a person reads before promoting.

--since <id> narrows it to what has been staged since that entry, which is how
a dispatcher asks what one slice recorded: note the last id in the queue before
dispatching, pass it afterwards, and refuse to advance on an empty answer
(--fail-if-empty exits 1 so a script does not have to parse for that).

--since is the boundary rather than a date or a slice because neither of those
can express it. Entry.Date is a day, so every capture in a session shares one
value. And --slice matches only requirements — 'capture --slice' is what MAKES
an entry a requirement, so a decision found while building a slice carries no
slice at all, and a slice's findings are mostly decisions. The queue is
append-only, so its own order is the honest timeline.`,
	Run: func(cmd *cobra.Command, args []string) {
		all, err := brain.NewQueue(brainProjectDir(cmd)).Load()
		if err != nil {
			brainFatal(err)
		}
		entries := all
		filter := brain.StagedFilter{}
		filter.SinceID, _ = cmd.Flags().GetString("since")
		filter.Slice, _ = cmd.Flags().GetString("slice")
		entries, err = brain.FilterStaged(entries, filter)
		if err != nil {
			brainFatal(err)
		}
		failIfEmpty, _ := cmd.Flags().GetBool("fail-if-empty")
		defer func() {
			if failIfEmpty && len(entries) == 0 {
				os.Exit(1)
			}
		}()
		if globalJSONFlag {
			brainJSON(stagedReport(all, entries, filter))
			return
		}
		if len(entries) == 0 {
			if filter.Empty() {
				fmt.Println("Nothing staged.")
			} else {
				// Distinguished on purpose: "nothing matched" is the answer a
				// dispatcher acts on, and reading it as "the queue is empty"
				// would hide entries a person still has to promote.
				fmt.Println("Nothing staged matching that filter.")
			}
			return
		}
		for _, e := range entries {
			fmt.Printf("%s  %-18s  %s\n", e.ID, shardLabel(e.Shard()), e.Title)
			if len(e.Anchors) > 0 {
				fmt.Printf("        %s\n", strings.Join(e.Anchors, " "))
			}
		}
		fmt.Printf("\n%d staged. Promote with 'mxcli brain promote <id>'.\n", len(entries))
	},
}

var brainPromoteCmd = &cobra.Command{
	Use:   "promote <id>",
	Short: "Write a staged entry into its shard (the human step)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := brainProjectDir(cmd)
		store, queue := brain.NewStore(dir), brain.NewQueue(dir)
		if !store.Exists() {
			brainFatal(fmt.Errorf("no store yet — run 'mxcli brain init' first"))
		}
		e, ok, err := queue.Get(args[0])
		if err != nil {
			brainFatal(err)
		}
		if !ok {
			brainFatal(fmt.Errorf("no staged entry with id %s", args[0]))
		}
		shard := e.Shard()
		if to, _ := cmd.Flags().GetString("to"); to != "" {
			shard = to
		}
		if err := store.Promote(e, shard); err != nil {
			brainFatal(err)
		}
		if _, err := queue.Drop(e.ID); err != nil {
			brainFatal(err)
		}
		fmt.Printf("Promoted %s into %s\n", e.ID, store.ShardPath(shard))
	},
}

var brainDropCmd = &cobra.Command{
	Use:   "drop <id>",
	Short: "Remove an entry from the queue or from its shard",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := brainProjectDir(cmd)
		if dropped, err := brain.NewQueue(dir).Drop(args[0]); err != nil {
			brainFatal(err)
		} else if dropped {
			fmt.Printf("Dropped %s from the queue.\n", args[0])
			return
		}
		shard, deletedFile, err := brain.NewStore(dir).Drop(args[0])
		if err != nil {
			brainFatal(err)
		}
		if shard == "" {
			brainFatal(fmt.Errorf("no entry with id %s, staged or committed", args[0]))
		}
		fmt.Printf("Dropped %s from %s\n", args[0], shardLabel(shard))
		if deletedFile {
			fmt.Printf("%s had no entries left and was removed.\n", shardLabel(shard))
		}
	},
}

var brainShowCmd = &cobra.Command{
	Use:   "show [shard]",
	Short: "Size and headroom per shard (computed, never written down)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		store := brain.NewStore(brainProjectDir(cmd))
		if !store.Exists() {
			fmt.Println("No store yet. Create one with 'mxcli brain init'.")
			return
		}
		usage, err := store.Usage()
		if err != nil {
			brainFatal(err)
		}
		// Width is computed from the names actually present: a module shard is
		// named after its module, and those run long.
		if len(args) == 1 {
			usage = slices.DeleteFunc(usage, func(u brain.Usage) bool { return u.Shard != args[0] })
		}
		if globalJSONFlag {
			brainJSON(map[string]any{"shards": usage})
			return
		}
		width := len("SHARD")
		for _, u := range usage {
			if n := len(shardLabel(u.Shard)); n > width {
				width = n
			}
		}
		fmt.Printf("%-*s %8s %8s %12s\n", width, "SHARD", "ENTRIES", "LINES", "HEADROOM")
		for _, u := range usage {
			note := ""
			if u.Over() {
				note = "  OVER CAP"
			}
			fmt.Printf("%-*s %8d %8d %7d/%-4d%s\n", width, shardLabel(u.Shard), u.Entries, u.Lines, u.Headroom(), u.Cap, note)
		}
	},
}

var brainResolveCmd = &cobra.Command{
	Use:   "resolve <id> <answer>",
	Short: "Answer an open question, turning it into a decision",
	Long: `Answer an open question.

The entry becomes an ordinary decision in place: same id, same position in the
file, with the question kept as the answer's context. It does not move and does
not get a new identity, because an answered question is the same piece of
knowledge as the question — anything referring to it still resolves.

This is the step that stops questions accumulating. A question nobody answers
is reported by 'brain check' and by 'mxcli lint' until someone does.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		store := brain.NewStore(brainProjectDir(cmd))
		e, shard, err := store.Find(args[0])
		if err != nil {
			brainFatal(err)
		}
		if shard == "" {
			brainFatal(fmt.Errorf("no committed entry with id %s", args[0]))
		}
		resolved, err := e.Resolve(args[1], time.Now())
		if err != nil {
			brainFatal(err)
		}
		if err := store.Replace(shard, resolved); err != nil {
			brainFatal(err)
		}
		fmt.Printf("Resolved %s in %s\n", resolved.ID, shardLabel(shard))
	},
}

var brainPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "The roadmap: each slice's requirements counted against the model",
	Long: `Show the plan — the slices, and how much of each is built.

Every figure is DERIVED. A requirement is "built" when its anchors resolve
against the model, so nothing is self-reported and there is no status column to
maintain or to go stale. Building the thing is what moves the number.

Slices are listed in name order, so a numeric prefix is how a roadmap is
sequenced: 01-accounts, 02-approvals. That is the user's choice, not a field
mxcli maintains.

The brain can exist before the project does. With no .mpr at the -p path,
nothing is built: every anchored requirement counts as planned, and the output
says the model is absent.`,
	Run: func(cmd *cobra.Command, args []string) {
		loc := brainLocationOf(cmd)
		store := brain.NewStore(loc.Dir)
		sliceShards, err := store.ListSlices()
		if err != nil {
			brainFatal(err)
		}
		if only, _ := cmd.Flags().GetString("slice"); only != "" {
			want := brain.PlanShard(only)
			if !slices.Contains(sliceShards, want) {
				brainFatal(fmt.Errorf("no slice %q; 'mxcli brain plan' lists them", only))
			}
			sliceShards = []string{want}
		}
		if len(sliceShards) == 0 {
			fmt.Println("No slices yet. Record one with:")
			fmt.Println("  mxcli brain capture \"<requirement>\" --slice 01-<name> -a @Module.Element")
			return
		}
		resolver, closeFn, err := openBrainResolverOrNoModel(loc.MPR)
		if err != nil {
			brainFatal(err)
		}
		defer closeFn()

		rep, err := brain.Check(store, resolver, sliceShards)
		if err != nil {
			brainFatal(err)
		}
		if globalJSONFlag {
			brainJSON(planReport(rep))
			return
		}
		if rep.ModelAbsent {
			printNoModel(rep, loc)
			fmt.Println()
		}
		printBrainPlan(rep.Slices)
	},
}

func printBrainPlan(slices []brain.SliceProgress) {
	width := len("SLICE")
	for _, sl := range slices {
		if n := len(sl.Slice); n > width {
			width = n
		}
	}
	var built, total int
	fmt.Printf("%-*s %8s %8s %12s\n", width, "SLICE", "BUILT", "PLANNED", "UNANCHORED")
	for _, sl := range slices {
		un := ""
		if sl.Unanchored > 0 {
			un = fmt.Sprint(sl.Unanchored)
		}
		fmt.Printf("%-*s %8d %8d %12s\n", width, sl.Slice, sl.Built, sl.Planned, un)
		built += sl.Built
		total += sl.Total()
	}
	fmt.Printf("\n%d of %d requirements built, across %d slice(s).\n", built, total, len(slices))
}

var brainBriefCmd = &cobra.Command{
	Use:   "brief",
	Short: "The reading pack for a slice: project + its modules + its plan",
	Long: `Emit exactly the shards a session needs, as one bounded read.

The store is sharded so a session can load project.md plus the modules it is
touching instead of the whole thing. Nothing produced that pack, though —
docs/brain/ is a directory, so a session either read all of it or guessed.

  mxcli brain brief --slice 07-planning     project + the modules that slice's
                                            requirements anchor into + its plan
  mxcli brain brief --module Sales --module Finance
                                            project + those modules, no plan
                                            (maintenance rather than roadmap)

Which modules a slice needs is DERIVED from its requirements' anchors, not
configured: asking the caller which modules its slice touches would be asking
it the thing it opened the brief to find out.

The pack goes to stdout and the size line to stderr, so it can be piped
straight into a prompt. --json gives the shards separately with their paths.`,
	Example: `  mxcli brain brief --slice 07-planning -p app.mpr
  mxcli brain brief --module Sales -p app.mpr --json`,
	Run: func(cmd *cobra.Command, args []string) {
		store := brain.NewStore(brainProjectDir(cmd))
		if !store.Exists() {
			fmt.Println("No store yet. Create one with 'mxcli brain init'.")
			return
		}
		slice, _ := cmd.Flags().GetString("slice")
		modules, _ := cmd.Flags().GetStringSlice("module")
		if slice == "" && len(modules) == 0 {
			brainFatal(fmt.Errorf("say what the session is working on: --slice <name> or --module <Module>"))
		}
		if slice != "" && len(modules) > 0 {
			// Refused rather than merged: a brief's value is what it leaves
			// out, and silently widening the pack past what was asked for is
			// the whole-store read it exists to replace.
			brainFatal(fmt.Errorf("--slice and --module are different questions; pass one"))
		}

		var (
			b   brain.Brief
			err error
		)
		if slice != "" {
			b, err = store.Brief(slice)
		} else {
			b, err = store.BriefForModules(modules)
		}
		if err != nil {
			brainFatal(err)
		}

		if globalJSONFlag {
			brainJSON(b)
			return
		}
		fmt.Print(b.Text())
		// stderr, so `brain brief | ...` pipes the pack and not the commentary.
		fmt.Fprintln(os.Stderr, b.Summary())
	},
}

var brainCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Do the anchors still resolve, and is every entry in the right shard?",
	Long: `Validate the store against the model.

Two independent questions. Each anchor is resolved, and reported as one of
three states — only "not found" is a failure. An anchor whose target exists but
is of a document type the catalog does not index is reported as NOT INDEXABLE
and passes: treating it as missing would demand edits to entries that are
perfectly current.

Separately, each entry is checked for being in the right shard. That is a second
axis, not a fourth state — an anchor can resolve perfectly and the entry still
sit in the wrong file. At least one anchor must belong to the entry's shard;
anchors into other modules are fine, because a fact can genuinely span two.

With no .mpr at the -p path there is nothing to resolve against: every anchor
is UNVERIFIABLE, which is counted and reported once and does not fail.`,
	Run: func(cmd *cobra.Command, args []string) {
		loc := brainLocationOf(cmd)
		store := brain.NewStore(loc.Dir)
		if !store.Exists() {
			fmt.Println("No store yet. Create one with 'mxcli brain init'.")
			return
		}
		shards, err := store.ListShards()
		if err != nil {
			brainFatal(err)
		}
		if changed, _ := cmd.Flags().GetBool("changed"); changed {
			shards, err = changedShards(loc.Dir, shards)
			if err != nil {
				brainFatal(err)
			}
			if len(shards) == 0 {
				fmt.Println("No brain shards changed.")
				return
			}
		}

		resolver, closeFn, err := openBrainResolverOrNoModel(loc.MPR)
		if err != nil {
			brainFatal(err)
		}
		defer closeFn()

		rep, err := brain.Check(store, resolver, shards)
		if err != nil {
			brainFatal(err)
		}
		switch ci, _ := cmd.Flags().GetBool("ci"); {
		case globalJSONFlag:
			// --json wins over --ci: both exist for a machine, and one of them
			// carries the states and counts rather than only the problems.
			brainJSON(rep)
		case ci:
			printBrainReportCI(rep)
		default:
			printBrainReport(rep, loc)
		}
		if rep.Failed() {
			os.Exit(1)
		}
	},
}

// printBrainReportCI emits one stable, greppable line per problem and nothing
// at all when the store is clean — what a CI log wants. Informational states
// (a not-indexable anchor) are omitted rather than printed, because a line in a
// CI log reads as something to fix.
func printBrainReportCI(rep brain.Report) {
	if rep.ModelAbsent {
		// stderr: in CI an absent model is worth seeing, but it is not a
		// problem line, and stdout stays empty for a clean store.
		fmt.Fprintf(os.Stderr, "brain: no model; %d anchor(s) not checked\n", rep.Unverifiable)
	}
	for _, m := range rep.Malformed {
		fmt.Printf("brain: malformed entry: %s\n", m)
	}
	for _, f := range rep.Findings {
		if f.State != brain.NotFound {
			continue
		}
		fmt.Printf("brain: %s: %s: anchor %s not found\n", shardLabel(f.Shard), f.EntryID, f.Anchor)
	}
	for _, m := range rep.Misfiled {
		fmt.Printf("brain: %s: %s: misfiled, resolves in %s\n", shardLabel(m.Shard), m.EntryID, m.Belongs)
	}
}

// printNoModel says, once, that nothing could be checked. Without it a plan of
// "0 built" reads as "nothing has been done" when the truth is "there is no
// project yet", and the two call for different actions.
func printNoModel(rep brain.Report, loc brainLocation) {
	if !rep.ModelAbsent {
		return
	}
	where := "at " + loc.MPR
	if loc.MPR == "" {
		where = "in " + loc.Dir
	}
	fmt.Printf("No model %s yet: %d anchor(s) not checked, so nothing counts as built.\n",
		where, rep.Unverifiable)
}

func printBrainReport(rep brain.Report, loc brainLocation) {
	printNoModel(rep, loc)
	for _, m := range rep.Malformed {
		fmt.Printf("MALFORMED   %s\n", m)
	}
	for _, f := range rep.Findings {
		label := "NOT FOUND  "
		if f.State == brain.NotIndexable {
			label = "not indexable"
		}
		fmt.Printf("%-13s %s  %s (%s: %q)\n", label, f.Anchor, shardLabel(f.Shard), f.EntryID, f.Title)
	}
	for _, m := range rep.Misfiled {
		belongs := m.Belongs
		if belongs == "" {
			belongs = "unknown"
		}
		fmt.Printf("MISFILED      %s: %q is in %s but resolves in %s\n",
			m.EntryID, m.Title, shardLabel(m.Shard), belongs)
	}
	if len(rep.Slices) > 0 {
		fmt.Println()
		printBrainPlan(rep.Slices)
	}
	for _, q := range rep.Open {
		fmt.Printf("OPEN          %s: %q (%s)\n", q.EntryID, q.Title, shardLabel(q.Shard))
	}
	fmt.Printf("\n%d entries, %d anchors, %d resolved, across %d shard(s).\n",
		rep.Entries, rep.Anchors, rep.ResolvedN, len(rep.Shards))
	if n := len(rep.Open); n > 0 {
		noun := "questions"
		if n == 1 {
			noun = "question"
		}
		fmt.Printf("%d open %s — answer one with 'mxcli brain resolve <id> \"<answer>\"'.\n", n, noun)
	}
	if !rep.Failed() {
		fmt.Println("OK")
	}
}

// changedShards narrows a check to the shards a diff touches. That is the cheap
// half of the CI answer: the catalog still has to be built once, but nothing
// pays to re-check shards nobody edited.
func changedShards(projectDir string, all []string) ([]string, error) {
	out, err := runGit(projectDir, "diff", "--name-only", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("--changed needs a git repository: %w", err)
	}
	staged, err := runGit(projectDir, "diff", "--name-only", "--cached")
	if err != nil {
		return nil, err
	}
	touched := map[string]bool{}
	for _, line := range strings.Split(out+"\n"+staged, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, brain.StoreDir+"/") {
			continue
		}
		touched[shardForPath(line)] = true
	}
	var shards []string
	for _, s := range all {
		if touched[s] {
			shards = append(shards, s)
		}
	}
	sort.Strings(shards)
	return shards, nil
}

// shardForPath maps a file under docs/brain/ back to its shard name. The
// basename alone is not enough: a plan slice's shard carries the plan/ prefix,
// so mapping docs/brain/plan/01-accounts.md to "01-accounts" made every edited
// slice invisible to --changed. Found by editing one; the unit test had only
// ever used module shards, which is exactly the blind spot.
func shardForPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".md")
	if strings.Contains(filepath.ToSlash(filepath.Dir(path)), brain.StoreDir+"/plan") {
		return brain.PlanShard(base)
	}
	return base
}

// catalogResolver answers anchors from the catalog, falling back to a
// type-agnostic unit lookup for the types the catalog's objects view does not
// cover. Without that fallback an anchor to such a document reads as missing,
// which is a false staleness signal (A1).
type catalogResolver struct {
	cat *catalog.Catalog
	be  backend.FullBackend
}

func (r *catalogResolver) Resolve(a brain.Anchor) (brain.Resolution, error) {
	if a.IsMember() {
		rows, err := r.query(fmt.Sprintf(
			"SELECT ModuleName FROM attributes_data WHERE EntityQualifiedName = '%s' AND Name = '%s' LIMIT 1",
			sqlLiteral(a.QualifiedName()), sqlLiteral(a.Member)))
		if err != nil {
			return brain.Resolution{}, err
		}
		if len(rows) == 1 {
			return brain.Resolution{State: brain.Resolved, Module: brainCell(rows[0][0]), Kind: "attribute"}, nil
		}
		// The member is gone. Whether its entity survives changes nothing: the
		// anchor as written names something that is not there.
		return brain.Resolution{State: brain.NotFound}, nil
	}

	rows, err := r.query(fmt.Sprintf(
		"SELECT ObjectType, ModuleName, Name FROM objects WHERE QualifiedName = '%s' LIMIT 1",
		sqlLiteral(a.QualifiedName())))
	if err != nil {
		return brain.Resolution{}, err
	}
	if len(rows) == 1 {
		objectType, moduleName, name := brainCell(rows[0][0]), brainCell(rows[0][1]), brainCell(rows[0][2])
		// A MODULE row carries no ModuleName — it IS the module — so the
		// misfiling comparison has to be given the module's own name.
		if objectType == "MODULE" {
			moduleName = name
		}
		return brain.Resolution{State: brain.Resolved, Module: moduleName, Kind: strings.ToLower(objectType)}, nil
	}

	if a.Element == "" {
		return brain.Resolution{State: brain.NotFound}, nil // a module is always indexed
	}
	if unit, err := r.be.FindDocumentUnit(a.Module, a.Element); err == nil && unit != nil {
		return brain.Resolution{State: brain.NotIndexable, Module: a.Module, Kind: unit.Kind}, nil
	}
	// A module ROLE is neither in the objects view nor a document, so both
	// lookups above miss it and the anchor reads as NotFound — a failure state.
	// For a requirement, whose anchors point forward, that means "not built
	// yet" FOREVER: the roles exist, the work is done, and `brain plan` still
	// reports it planned (ako/ChipCoV1). Roles are a natural thing to anchor a
	// security requirement at, so the resolver has to be able to see them.
	if ok, err := r.moduleRoleExists(a.Module, a.Element); err == nil && ok {
		return brain.Resolution{State: brain.Resolved, Module: a.Module, Kind: "module role"}, nil
	}
	return brain.Resolution{State: brain.NotFound}, nil
}

// moduleRoleExists reports whether the module declares a role of that name.
//
// The comparison is case-insensitive because Mendix treats role names that way
// and an anchor is written by hand; a case-only mismatch reporting "not found"
// would be the same false staleness this whole file exists to avoid.
func (r *catalogResolver) moduleRoleExists(moduleName, roleName string) (bool, error) {
	mod, err := r.be.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return false, nil // an unknown module is NotFound, not an error
	}
	sec, err := r.be.GetModuleSecurity(mod.ID)
	if err != nil || sec == nil {
		return false, err
	}
	for _, role := range sec.ModuleRoles {
		if role != nil && strings.EqualFold(role.Name, roleName) {
			return true, nil
		}
	}
	return false, nil
}

func (r *catalogResolver) query(sql string) ([][]any, error) {
	res, err := r.cat.Query(sql)
	if err != nil {
		return nil, err
	}
	return res.Rows, nil
}

// sqlLiteral escapes a value for a SQL string literal. Anchors are validated as
// Mendix identifiers before they get here, so nothing can reach this with a
// quote in it — the escape is belt and braces, not the guard.
func sqlLiteral(s string) string { return strings.ReplaceAll(s, "'", "''") }

func brainCell(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// openBrainResolverOrNoModel opens the project, or answers for a project that
// does not exist yet. Only a missing file means "no model": an .mpr that is
// there but will not open is still an error, or a corrupt project would be
// reported as merely unbuilt.
func openBrainResolverOrNoModel(projectPath string) (brain.Resolver, func(), error) {
	if projectPath == "" {
		return brain.NoModel{}, func() {}, nil // -C on a directory with no .mpr yet
	}
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return brain.NoModel{}, func() {}, nil
	}
	return openBrainResolver(projectPath)
}

func openBrainResolver(projectPath string) (brain.Resolver, func(), error) {
	// Catalog progress goes to stderr so stdout carries only the report — the
	// same split cmd_lint.go makes, and what lets `brain check` be piped.
	exec, logger := newLoggedExecutorTo("subcommand", os.Stderr)
	cleanup := func() { logger.Close(); exec.Close() }

	for _, src := range []string{
		fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)),
		"REFRESH CATALOG",
	} {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			cleanup()
			return nil, nil, errs[0]
		}
		for _, stmt := range prog.Statements {
			if err := exec.Execute(stmt); err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("%s: %w", src, err)
			}
		}
	}
	return &catalogResolver{cat: exec.Catalog(), be: exec.Backend()}, cleanup, nil
}

// runGit shells out in the project directory. Only used by --changed, where the
// question is which files the working tree has touched.
func runGit(dir string, args ...string) (string, error) {
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func shardLabel(shard string) string {
	switch {
	case shard == brain.ProjectShard:
		return "project.md"
	case brain.IsPlanShard(shard):
		return "plan/" + brain.SliceOf(shard) + ".md"
	default:
		return shard + ".md"
	}
}

// brainLocation is where a brain lives and which model, if any, it is checked
// against.
//
// The store is in Dir; the .mpr is only ever needed by plan and check. That
// split is deliberate and load-bearing (ako/mxcli#1949): a brain is routinely
// started before its project exists — requirements first, app second — so no
// store command may open, or even stat, the .mpr. MPR is empty when -C names a
// directory that holds no project yet.
type brainLocation struct {
	Dir string
	MPR string
}

// resolveBrainLocation turns -p / -C into a location.
//
//   - -p <app.mpr> is the shorthand: the directory that .mpr is (or will be)
//     in. The file need not exist.
//   - -C <dir> names the directory. The model is the one .mpr in it, if there
//     is one, so the same command keeps working after `mxcli new` creates the
//     project there; more than one is refused rather than guessed between.
func resolveBrainLocation(project, dir string) (brainLocation, error) {
	switch {
	case project != "" && dir != "":
		return brainLocation{}, fmt.Errorf("pass -p <app.mpr> or -C <dir>, not both")
	case project != "":
		return brainLocation{Dir: filepath.Dir(project), MPR: project}, nil
	case dir != "":
		matches, err := filepath.Glob(filepath.Join(dir, "*.mpr"))
		if err != nil {
			return brainLocation{}, err
		}
		switch len(matches) {
		case 0:
			return brainLocation{Dir: dir}, nil
		case 1:
			return brainLocation{Dir: dir, MPR: matches[0]}, nil
		default:
			names := make([]string, len(matches))
			for i, m := range matches {
				names[i] = filepath.Base(m)
			}
			return brainLocation{}, fmt.Errorf("%s holds more than one project (%s); name one with -p",
				dir, strings.Join(names, ", "))
		}
	default:
		return brainLocation{}, fmt.Errorf("say where the brain is: -p <app.mpr> or -C <dir>")
	}
}

func brainLocationOf(cmd *cobra.Command) brainLocation {
	project, _ := cmd.Flags().GetString("project")
	dir, _ := cmd.Flags().GetString("dir")
	loc, err := resolveBrainLocation(project, dir)
	if err != nil {
		brainFatal(err)
	}
	return loc
}

func brainProjectDir(cmd *cobra.Command) string { return brainLocationOf(cmd).Dir }

func brainFatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

// brainLocatedCommands is every brain command that needs to know where the
// brain is — all of them, which is why -p and -C are registered from one list.
func brainLocatedCommands() []*cobra.Command {
	return []*cobra.Command{
		brainInitCmd, brainCaptureCmd, brainStagedCmd, brainPromoteCmd,
		brainDropCmd, brainShowCmd, brainCheckCmd, brainPlanCmd,
		brainResolveCmd, brainBriefCmd,
	}
}

func init() {
	for _, c := range brainLocatedCommands() {
		c.Flags().StringP("project", "p", "", "Path to the .mpr file (need not exist yet; the brain lives beside it)")
		c.Flags().StringP("dir", "C", "", "Directory holding the brain; its one .mpr, if any, is the model")
	}
	brainCaptureCmd.Flags().StringSliceP("anchor", "a", nil,
		"Anchor into the model (@Module, @Module.Element, @Module.Entity.Attribute); repeatable")
	brainCaptureCmd.Flags().Bool("open", false,
		"Record this as an OPEN QUESTION — something not decided yet, so its anchors are not checked")
	brainCaptureCmd.Flags().StringP("slice", "s", "",
		"Record this as a requirement of the named slice (plan/<slice>.md) instead of a decision")
	brainPromoteCmd.Flags().String("to", "",
		"Override the derived shard (use 'project' for a cross-cutting fact)")
	brainStagedCmd.Flags().String("since", "",
		"Only entries staged after this entry id — the slice boundary (see 'mxcli brain staged --help')")
	brainStagedCmd.Flags().String("slice", "",
		"Only requirements of this slice (decisions carry no slice; use --since for a slice's findings)")
	brainStagedCmd.Flags().Bool("fail-if-empty", false,
		"Exit 1 when nothing matches, so a dispatcher can refuse to advance on a slice that recorded nothing")
	brainCheckCmd.Flags().Bool("changed", false, "Only check shards touched by the working tree")
	brainCheckCmd.Flags().Bool("ci", false, "Machine-friendly output for CI")

	brainPlanCmd.Flags().String("slice", "",
		"Report only this slice, instead of every slice in the plan")
	brainBriefCmd.Flags().String("slice", "",
		"The slice being worked; its modules are derived from its requirements' anchors")
	brainBriefCmd.Flags().StringSlice("module", nil,
		"Modules being worked, for a session with no slice; repeatable")
	brainCmd.AddCommand(brainInitCmd, brainCaptureCmd, brainStagedCmd,
		brainPromoteCmd, brainDropCmd, brainShowCmd, brainCheckCmd, brainPlanCmd,
		brainResolveCmd, brainBriefCmd)
	rootCmd.AddCommand(brainCmd)
}

// brainJSON writes v to stdout as indented JSON. Every brain command printed
// for a human only, which is what made the store unusable as the channel
// between sub-agents: an orchestrator dispatching one agent per slice has to
// DECIDE on `staged` and `check`, not read them.
//
// Indented on purpose. These outputs are small (a queue, a plan, a report), and
// the consumer is as often a person eyeballing what the machine will see as a
// parser.
func brainJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		brainFatal(err)
	}
	fmt.Println(string(b))
}

// stagedEntry is one queued entry as a machine sees it. The shard is included
// because it is derived (an entry's first anchor names its file), so a caller
// would otherwise have to reimplement the routing rule to know where a promote
// would put it.
type stagedEntry struct {
	brain.Entry
	Shard string `json:"shard"`
}

// stagedReport renders the queue for a machine. `all` is the unfiltered queue
// and `shown` what survived the filter — both are needed, because the two
// figures a dispatcher wants come from different sides of it.
func stagedReport(all, shown []brain.Entry, filter brain.StagedFilter) map[string]any {
	out := make([]stagedEntry, 0, len(shown))
	for _, e := range shown {
		out = append(out, stagedEntry{Entry: e, Shard: e.Shard()})
	}
	rep := map[string]any{"staged": out, "count": len(out)}

	// The id to pass as --since next time. It comes from the UNFILTERED queue
	// and is reported even when nothing matched, which is exactly the case a
	// dispatcher needs it in: a slice that staged nothing must still hand the
	// next slice a boundary, or the next one re-reports this one's captures.
	if len(all) > 0 {
		rep["last_id"] = all[len(all)-1].ID
	}
	rep["queue_size"] = len(all)

	// A count of 0 means two different things and the number cannot say which:
	// the queue is empty, or the filter matched nothing.
	if !filter.Empty() {
		rep["filtered"] = true
		if filter.SinceID != "" {
			rep["since"] = filter.SinceID
		}
		if filter.Slice != "" {
			rep["slice"] = filter.Slice
		}
	}
	return rep
}

// planReport carries the totals as well as the slices. A dispatcher's question
// is usually "is this slice done", and the answer is a comparison it should not
// have to assemble from four counters.
func planReport(rep brain.Report) map[string]any {
	var built, total int
	for _, sl := range rep.Slices {
		built += sl.Built
		total += sl.Total()
	}
	out := map[string]any{
		"slices": rep.Slices,
		"built":  built,
		"total":  total,
	}
	if rep.ModelAbsent {
		out["model_absent"] = true
	}
	return out
}
