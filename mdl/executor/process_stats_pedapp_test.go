// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// readProcessStats returns the lines a stats file holds.
func readProcessStats(t *testing.T, path string) []ProcessStats {
	t.Helper()
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []ProcessStats
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var s ProcessStats
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			t.Fatalf("stats line %q: %v", sc.Text(), err)
		}
		out = append(out, s)
	}
	return out
}

// A connection reports what it read and wrote when it closes, so a benchmark
// timing whole mxcli processes can also record counts that do not depend on
// the machine (PROPOSAL_agent_devloop_benchmark.md). The three connections
// below are told apart by what they did: a read-only one, one that writes, and
// one that offers the same write again and lands nothing.
func TestProcessStats_OneLinePerConnection(t *testing.T) {
	stats := filepath.Join(t.TempDir(), "stats.jsonl")
	t.Setenv(StatsFileEnv, stats)

	const create = "mdl 1;\ncreate or modify persistent entity MyFirstModule.StatsProbe ( Name: string(50) );"
	exec, out, dir := openPedAppCopy(t)
	run := func(src string) {
		t.Helper()
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("%s: %v\n%s", src, err, out.String())
		}
		if err := exec.Execute(&ast.DisconnectStmt{}); err != nil {
			t.Fatal(err)
		}
		if err := exec.Execute(&ast.ConnectStmt{Path: filepath.Join(dir, "PedApp.mpr")}); err != nil {
			t.Fatal(err)
		}
	}
	run("show modules;")
	run(create)
	run(create)

	got := readProcessStats(t, stats)
	if len(got) != 3 {
		t.Fatalf("got %d stats lines, want one per closed connection (3): %+v", len(got), got)
	}
	if got[0].UnitFileReads == 0 {
		t.Errorf("show modules read no unit files: %+v — the count is taken after the reader is gone", got[0])
	}
	if got[0].WritesWritten != 0 {
		t.Errorf("show modules wrote: %+v", got[0])
	}
	if got[1].WritesWritten == 0 {
		t.Errorf("creating an entity landed no write: %+v — the count is taken before the deferred writes flush", got[1])
	}
	if got[2].WritesOffered == 0 || got[2].WritesWritten != 0 {
		t.Errorf("re-running the same statement: %+v, want writes offered and none landed", got[2])
	}
}
