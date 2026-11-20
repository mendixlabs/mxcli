// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// ako/mxcli#1860: off a terminal a catalog build prints none of its routine
// lines while it is quick, and the latest one at most every interval while it
// is slow, so a minutes-long build still shows it is alive.
func TestCatalogHeartbeatThrottlesRoutineLines_Issue1860(t *testing.T) {
	var out bytes.Buffer
	clock := time.Unix(0, 0)
	h := newCatalogHeartbeat(&out)
	h.now = func() time.Time { return clock }
	h.start, h.last = clock, clock

	for _, table := range []string{"Modules", "Entities", "Attributes"} {
		clock = clock.Add(time.Second)
		h.line(table, 7)
	}
	if out.Len() != 0 {
		t.Fatalf("a 3-second build printed:\n%s", out.String())
	}

	clock = clock.Add(13 * time.Second) // 16 s in
	h.line("Microflows", 42)
	clock = clock.Add(2 * time.Second)
	h.line("source 10/300 documents", 10)
	clock = clock.Add(14 * time.Second) // 32 s in
	h.line("source 250/300 documents", 250)

	got := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{
		"… building catalog, Microflows: 42 (16s)",
		"… building catalog, source 250/300 documents (32s)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("heartbeat:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
