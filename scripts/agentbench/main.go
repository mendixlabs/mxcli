// SPDX-License-Identifier: Apache-2.0

// Command agentbench measures how long mxcli takes to do what an agent asks of
// it while building an app: a fixed, scripted session of mxcli commands, one
// process per command, with no model in the loop.
//
// Given a second binary (-baseline) it runs both interleaved on the same
// machine and reports the ratio, which is what makes a figure from a shared CI
// runner comparable from one night to the next. It also records counts that do
// not depend on the machine at all: unit files read and unit writes landed.
//
//	make bench                                   # the binary in bin/
//	make bench BASELINE=/path/to/another/mxcli   # A/B against another build
//
// Design and reading guide: docs/11-proposals/PROPOSAL_agent_devloop_benchmark.md
// and docs-site/src/internals/agent-benchmark.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var cfg config
	var sizes, out, summary string
	flag.StringVar(&cfg.candidate, "mxcli", "bin/mxcli", "the mxcli to measure")
	flag.StringVar(&cfg.baseline, "baseline", "", "a second mxcli to compare against, interleaved on this machine (optional)")
	flag.StringVar(&cfg.base, "base", "testdata/pedapp", "directory of the project every session starts from (MPR v2, one .mpr)")
	flag.StringVar(&cfg.work, "work", "", "working directory (default: a temporary one, removed afterwards)")
	flag.IntVar(&cfg.reps, "reps", 5, "repetitions of the session per arm; the median is reported")
	flag.IntVar(&cfg.sessionEntities, "session-entities", 25, "entities the session authors (each brings 2 microflows and 2 pages)")
	flag.IntVar(&cfg.large.Modules, "large-modules", 12, "modules seeded into the large project")
	flag.IntVar(&cfg.large.Entities, "large-entities", 10, "entities per seeded module (each brings 2 microflows and 2 pages)")
	flag.IntVar(&cfg.scaleMicroflows, "scale-microflows", 4000, "filler microflows seeded into the scale project")
	flag.IntVar(&cfg.scaleBatch, "scale-batch", 100, "statements in each batch of the scale session")
	flag.StringVar(&sizes, "sizes", "small,large,scale", "sessions to run: small and large are the scripted session at two project sizes, scale is per-statement cost on a project of thousands of units")
	flag.StringVar(&cfg.runtimeVersion, "runtime-version", "", "also run the running-app session (boot, hot apply, tests) on a project created by `mxcli new` for this Mendix version, e.g. 11.13.0; installs mxbuild and the runtime if they are not cached")
	flag.IntVar(&cfg.runtimeReps, "runtime-reps", 3, "repetitions of the running-app session per arm")
	flag.BoolVar(&cfg.pageCheck, "page-check", true, "include the page-check step in the running-app session (needs node with playwright and a Chromium)")
	flag.Float64Var(&cfg.threshold, "threshold", 1.20, "wall-time ratio above which a step is flagged slower (and below its inverse, faster)")
	flag.StringVar(&out, "out", "", "write the JSON record here")
	flag.StringVar(&summary, "summary", "", "append the Markdown summary here (e.g. $GITHUB_STEP_SUMMARY); it is always printed")
	flag.Parse()

	cfg.large.Prefix = "Seed"
	if sizes != "" {
		cfg.sizes = strings.Split(sizes, ",")
	}
	cfg.log = os.Stderr
	if cfg.scaleMicroflows < 1 || cfg.scaleBatch < 1 {
		fatal(errors.New("-scale-microflows and -scale-batch must be at least 1"))
	}
	if cfg.reps < 1 || cfg.runtimeReps < 1 {
		fatal(errors.New("-reps and -runtime-reps must be at least 1"))
	}
	if cfg.work == "" {
		dir, err := os.MkdirTemp("", "agentbench-")
		if err != nil {
			fatal(err)
		}
		defer os.RemoveAll(dir)
		cfg.work = dir
	} else {
		abs, err := filepath.Abs(cfg.work)
		if err != nil {
			fatal(err)
		}
		cfg.work = abs
		if err := os.MkdirAll(cfg.work, 0o755); err != nil {
			fatal(err)
		}
	}
	// A relative binary path would be resolved against each child's directory.
	var err error
	if cfg.candidate, err = absBinary(cfg.candidate); err != nil {
		fatal(err)
	}
	if cfg.baseline != "" {
		if cfg.baseline, err = absBinary(cfg.baseline); err != nil {
			fatal(err)
		}
	}

	// A session that fails does not take the completed ones with it: their
	// record is written, and the run still ends in error.
	sessions, runErr := run(cfg)
	if runErr != nil && len(sessions) == 0 {
		fatal(runErr)
	}
	rec := newRecord(cfg, sessions)
	md := rec.markdown()
	fmt.Print(md)
	if summary != "" {
		f, err := os.OpenFile(summary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			fatal(err)
		}
		_, _ = f.WriteString(md)
		f.Close()
	}
	if out != "" {
		if err := rec.write(out); err != nil {
			fatal(err)
		}
	}
	if runErr != nil {
		// The record is written first: the night an invariant breaks is the
		// night its numbers are wanted.
		fatal(runErr)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "agentbench:", err)
	os.Exit(1)
}
