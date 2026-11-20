// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// catalogHeartbeatEvery is how often a catalog build that is not writing to a
// terminal says it is still going.
const catalogHeartbeatEvery = 15 * time.Second

// catalogHeartbeat throttles a catalog build's routine progress lines — one
// "✓ Table: N" per table, the source pass's "source n/m documents" every 2 s —
// for a reader who is not watching a terminal.
//
// Printed in full they were about fifty lines on every build, and a build runs
// on the first command after each model change, so an agent paid for them in
// context over and over (ako/mxcli#1860). Dropping them is not the answer
// either: a cold build of a large project takes minutes, and a log with
// nothing in it for that long reads as a hang. So a build that finishes
// quickly prints none of them, and a slow one prints the latest at most every
// catalogHeartbeatEvery.
type catalogHeartbeat struct {
	w     io.Writer
	every time.Duration
	now   func() time.Time

	mu    sync.Mutex
	start time.Time
	last  time.Time
}

func newCatalogHeartbeat(w io.Writer) *catalogHeartbeat {
	h := &catalogHeartbeat{w: w, every: catalogHeartbeatEvery, now: time.Now}
	h.start = h.now()
	h.last = h.start
	return h
}

// line is a catalog.ProgressFunc.
func (h *catalogHeartbeat) line(text string, count int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.now()
	if t.Sub(h.last) < h.every {
		return
	}
	h.last = t
	fmt.Fprintf(h.w, "… building catalog, %s (%s)\n", progressText(text, count), formatDuration(t.Sub(h.start)))
}

// progressText is a routine progress line as the per-table form prints it:
// "Entities: 111", or the line itself when it already carries its numbers.
func progressText(text string, count int) string {
	for _, r := range text {
		if r >= '0' && r <= '9' {
			return text
		}
	}
	return fmt.Sprintf("%s: %d", text, count)
}

// isTerminal reports whether w writes to a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
