// SPDX-License-Identifier: Apache-2.0

// mask-matrix measures which errors a Mendix checker hides until others are fixed.
//
// It writes one microflow per defect and one per ordered pair of defects into a copy
// of a healthy project, runs `mx check` (and, optionally, Mendix CLI 2's `mendix
// check`) once over all of them, and reports, from what each checker says about the
// pairs, which defect hides which, the stages that gives, and whether the order inside
// a document matters. A second, smaller run shows what a reference that does not
// resolve hides across the whole app.
//
//	go run ./scripts/maskmatrix --project /path/to/healthy/App.mpr \
//	    --mx ~/.mxcli/mxbuild/11.16.1/modeler/mx [--mendix /path/to/mendix]
//
// The project must be healthy (it must check clean) and built with the Mendix version
// of the --mx you pass. See docs/11-proposals/PROPOSAL_check_diagnostics_catalog.md
// for what this found on MxBuild 11.16.1.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "mask-matrix:", err)
		os.Exit(1)
	}
}

type config struct {
	project, mx, mxcli, mendix string
	defects                    []Defect
	probe, keep                bool
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("mask-matrix", flag.ContinueOnError)
	fs.SetOutput(stderr)
	project := fs.String("project", "", "a healthy Mendix project (.mpr) to copy; required")
	mx := fs.String("mx", "", "path to MxBuild's modeler/mx; required")
	mxcli := fs.String("mxcli", "./bin/mxcli", "mxcli binary used to write the documents")
	mendix := fs.String("mendix", "", "path to Mendix CLI 2's `mendix`, to compare (optional)")
	only := fs.String("defects", "", "comma-separated subset of "+strings.Join(defectNames(Defects), ",")+" (default: all)")
	noProbe := fs.Bool("no-reference-probe", false, "skip the run that shows what an unresolved reference hides")
	keep := fs.Bool("keep", false, "keep the working copies and print where they are")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *project == "" || *mx == "" {
		return fmt.Errorf("--project and --mx are required")
	}
	defects, err := selectDefects(*only)
	if err != nil {
		return err
	}
	c := config{project: *project, mx: *mx, mxcli: *mxcli, mendix: *mendix, defects: defects, probe: !*noProbe, keep: *keep}
	return c.execute(stdout)
}

func defectNames(ds []Defect) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Name
	}
	return out
}

func selectDefects(only string) ([]Defect, error) {
	if only == "" {
		return Defects, nil
	}
	var out []Defect
	for _, n := range strings.Split(only, ",") {
		n = strings.TrimSpace(n)
		found := false
		for _, d := range Defects {
			if d.Name == n {
				out = append(out, d)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown defect %q (have %s)", n, strings.Join(defectNames(Defects), ", "))
		}
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("need at least two defects to compare")
	}
	return out, nil
}

func (c config) execute(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "maskmatrix-")
	if err != nil {
		return err
	}
	if c.keep {
		fmt.Fprintf(w, "working copies kept in %s\n\n", tmp)
	} else {
		defer os.RemoveAll(tmp)
	}
	mpr := filepath.Base(c.project)
	base := filepath.Join(tmp, "base")
	if err := copyProject(filepath.Dir(c.project), base); err != nil {
		return fmt.Errorf("copy the project: %w", err)
	}

	defects, gen, refused, err := c.write(tmp, base, mpr)
	if err != nil {
		return err
	}
	for _, r := range refused {
		fmt.Fprintf(w, "left out %s: mxcli refuses to write it even under --no-check\n", r)
	}
	fmt.Fprintf(w, "%d documents (%d defects, every ordered pair, and a baseline) in a copy of %s\n\n", len(gen.Docs)+1, len(defects), mpr)

	checkers := []struct{ name, bin string }{{"MxBuild mx check", c.mx}}
	if c.mendix != "" {
		checkers = append(checkers, struct{ name, bin string }{"mendix check (CLI 2)", c.mendix})
	}
	for _, ch := range checkers {
		f, err := runChecker(ch.bin, base, mpr, filepath.Join(tmp, "run-"+strings.Fields(ch.name)[0]))
		if err != nil {
			return fmt.Errorf("%s: %w", ch.name, err)
		}
		if len(f["Z_baseline"]) > 0 {
			fmt.Fprintf(w, "WARNING: %s reports errors on the clean baseline document; the project is not healthy\n", ch.name)
		}
		fmt.Fprintf(w, "=== %s\n", ch.name)
		Report(w, defects, Analyze(defects, gen.Docs, f))
	}
	if c.probe {
		if err := c.referenceProbe(w, tmp, mpr); err != nil {
			return fmt.Errorf("reference probe: %w", err)
		}
	}
	return nil
}

// write generates the documents and writes them into base with mxcli, leaving out any
// defect exec refuses (a rule it enforces even under --no-check rolls the whole script
// back, so the script is generated again without it).
func (c config) write(tmp, base, mpr string) ([]Defect, Generated, []string, error) {
	defects := append([]Defect(nil), c.defects...)
	var refused []string
	for range c.defects {
		gen := Generate(defects)
		script := filepath.Join(tmp, "m.mdl")
		if err := os.WriteFile(script, []byte(gen.Script), 0o644); err != nil {
			return nil, gen, nil, err
		}
		out, err := exec.Command(c.mxcli, "exec", script, "-p", filepath.Join(base, mpr), "--no-check").CombinedOutput()
		if err == nil {
			return defects, gen, refused, nil
		}
		d := Refused(string(out))
		if d == "" {
			return nil, gen, nil, fmt.Errorf("mxcli exec failed: %v\n%s", err, tail(string(out), 6))
		}
		refused = append(refused, d)
		var keep []Defect
		for _, x := range defects {
			if x.Name != d {
				keep = append(keep, x)
			}
		}
		defects = keep
		if len(defects) < 2 {
			return nil, gen, nil, fmt.Errorf("fewer than two defects can be written")
		}
	}
	return nil, Generated{}, nil, fmt.Errorf("could not write the documents")
}

// runChecker runs a checker on a fresh copy of dir (a checker may touch the project)
// and returns its findings per microflow.
func runChecker(bin, dir, mpr, work string) (Findings, error) {
	if err := copyProject(dir, work); err != nil {
		return nil, err
	}
	out := filepath.Join(filepath.Dir(work), filepath.Base(work)+".json")
	cmd := exec.Command(bin, "check", mpr, "-j", out)
	cmd.Dir = work
	cmd.Env = checkerEnv()
	// A checker exits non-zero when it finds errors; the JSON is the result.
	if b, err := cmd.CombinedOutput(); err != nil {
		if _, statErr := os.Stat(out); statErr != nil {
			return nil, fmt.Errorf("%v\n%s", err, tail(string(b), 6))
		}
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	return ParseFindings(raw)
}

// checkerEnv preloads the system FreeType on Linux, as `mxcli docker check` does:
// the JVM's own copy lacks a symbol Skia needs, so a bundled mx can abort without it.
func checkerEnv() []string {
	env := os.Environ()
	if runtime.GOOS != "linux" || os.Getenv("LD_PRELOAD") != "" {
		return env
	}
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	if arch == "" {
		return env
	}
	lib := "/usr/lib/" + arch + "-linux-gnu/libfreetype.so.6"
	if _, err := os.Stat(lib); err == nil {
		env = append(env, "LD_PRELOAD="+lib)
	}
	return env
}

var docRE = regexp.MustCompile(`^Microflow '(\w+)'`)

// ParseFindings reads the JSON a checker writes with -j: errors[].locations[] with the
// document and element each error is on. Errors on anything but a microflow are kept
// under their own document name.
func ParseFindings(raw []byte) (Findings, error) {
	var doc struct {
		Errors []struct {
			Code      string `json:"code"`
			Locations []struct {
				Document string `json:"document-name"`
				Element  string `json:"element-name"`
			} `json:"locations"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("not a checker report: %w", err)
	}
	f := Findings{}
	for _, e := range doc.Errors {
		for _, l := range e.Locations {
			name := l.Document
			if m := docRE.FindStringSubmatch(name); m != nil {
				name = m[1]
			}
			f[name] = append(f[name], Finding{Code: e.Code, Element: l.Element})
		}
	}
	return f, nil
}

// Report prints what Analyze found.
func Report(w io.Writer, defects []Defect, r Result) {
	fmt.Fprintln(w, "each defect alone:")
	for _, d := range r.Defects {
		fmt.Fprintf(w, "  %-9s %s\n", d, strings.Join(r.Signature[d], ", ")+silentNote(r, d))
	}
	if len(r.SharedCodes) > 0 {
		fmt.Fprintf(w, "  note: %s are raised by more than one defect, so those defects cannot be told apart\n", strings.Join(r.SharedCodes, ", "))
	}
	fmt.Fprintln(w, "\nwho hides whom (the same in both orders):")
	any := false
	for _, d := range r.Defects {
		if hs := r.Hides[d]; len(hs) > 0 {
			fmt.Fprintf(w, "  %-9s hides %s\n", d, strings.Join(hs, ", "))
			any = true
		}
	}
	if !any {
		fmt.Fprintln(w, "  nothing: every defect is reported alongside every other")
	}
	if len(r.OrderDependent) > 0 {
		fmt.Fprint(w, "order-dependent pairs:")
		for _, p := range r.OrderDependent {
			fmt.Fprintf(w, " %s/%s", p[0], p[1])
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintln(w, "order inside a document: no pair differs when swapped")
	}
	fmt.Fprintln(w, "\nstages (a defect in an earlier stage hides the later ones; one stage is reported together):")
	for i, st := range r.Stages {
		fmt.Fprintf(w, "  %d  %s\n", i+1, strings.Join(st, ", "))
	}
	fmt.Fprintln(w)
}

func silentNote(r Result, d string) string {
	if contains(r.Silent, d) {
		return "(clean: the checker does not report this defect, so it is left out of the comparison)"
	}
	return ""
}

// referenceProbe shows what a reference that does not resolve hides across the app:
// one document with such a reference, one with an undefined variable and one with a
// type error, checked before and after the reference defect is dropped.
func (c config) referenceProbe(w io.Writer, tmp, mpr string) error {
	probe := filepath.Join(tmp, "probe")
	if err := copyProject(filepath.Dir(c.project), probe); err != nil {
		return err
	}
	script := filepath.Join(tmp, "probe.mdl")
	body := `mdl 1;
create module Pr;
create microflow Pr.Helper ($Text: String) returns String begin return $Text; end;
create microflow Pr.M_ref () returns Integer begin $p = call microflow Pr.Helper(Wrong = 'a'); return 1; end;
create microflow Pr.M_undef () returns Integer begin declare $u Integer = $Nope; return 1; end;
create microflow Pr.M_type () returns Integer begin declare $t Integer = 'abc'; return 1; end;
`
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		return err
	}
	if out, err := exec.Command(c.mxcli, "exec", script, "-p", filepath.Join(probe, mpr), "--no-check").CombinedOutput(); err != nil {
		return fmt.Errorf("mxcli exec: %v\n%s", err, tail(string(out), 6))
	}
	show := func(label, work string) error {
		f, err := runChecker(c.mx, probe, mpr, work)
		if err != nil {
			return err
		}
		var parts []string
		for _, doc := range []string{"M_ref", "M_undef", "M_type"} {
			for _, x := range f[doc] {
				parts = append(parts, fmt.Sprintf("%s@%s", x.Code, doc))
			}
		}
		sort.Strings(parts)
		if len(parts) == 0 {
			parts = []string{"clean"}
		}
		fmt.Fprintf(w, "  %-34s %s\n", label, strings.Join(parts, ", "))
		return nil
	}
	fmt.Fprintln(w, "=== what an unresolved reference hides (MxBuild mx check)")
	if err := show("with the reference defect (M_ref):", filepath.Join(tmp, "probe-with")); err != nil {
		return err
	}
	drop := filepath.Join(tmp, "drop.mdl")
	if err := os.WriteFile(drop, []byte("mdl 1;\ndrop microflow Pr.M_ref;\n"), 0o644); err != nil {
		return err
	}
	if out, err := exec.Command(c.mxcli, "exec", drop, "-p", filepath.Join(probe, mpr)).CombinedOutput(); err != nil {
		return fmt.Errorf("mxcli exec (drop): %v\n%s", err, tail(string(out), 6))
	}
	return show("after dropping it:", filepath.Join(tmp, "probe-without"))
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// skipAtTop are entries of a project's root that a checker never reads and that are
// large: `mxcli new` puts a copy of the mxcli binary there (110 MB), and every working
// copy would repeat it.
var skipAtTop = map[string]bool{"mxcli": true, "mxcli.exe": true, ".git": true}

// copyProject copies a Mendix project directory to dst, leaving out skipAtTop and
// anything that is not a plain file or directory.
func copyProject(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if skipAtTop[strings.Split(filepath.ToSlash(rel), "/")[0]] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case !d.Type().IsRegular():
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
