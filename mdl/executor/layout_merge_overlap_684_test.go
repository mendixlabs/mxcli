// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// `mxcli layout flows` left merges on top of their neighbours on the main path
// (ako/mxcli#684): "'(merge)' (2355,80) and '(merge)' (2360,80) ACT_DealGame 5px",
// "'(merge)' (2815,1730) and '(unnamed)' (2740,1730) ACT_Refresh 75px" — a merge
// 5px into the activity before it, and two merges 5px apart. Both are an IF's
// closing merge placed from a branch measured short: a decision inside the branch
// was measured from its split's centre (ako/mxcli#1773). The reporter's project is
// not available, so these are the shapes that put merges next to each other:
// decisions that close at the same point, a decision followed by an activity in a
// branch, and both inside loops, nested loops included.
var consecutiveMergeShapes = []struct{ name, body string }{
	{"an if/else closing where its enclosing if closes", `
  if $Flag then
    log info node 'M' 'a';
    if $Other then
      log info node 'M' 'b';
    else
      log info node 'M' 'c';
    end if;
  else
    log info node 'M' 'd';
  end if;
  log info node 'M' 'done';`},
	{"an if without else closing where its enclosing if closes", `
  if $Flag then
    log info node 'M' 'a';
    if $Other then
      log info node 'M' 'b';
    end if;
  end if;
  log info node 'M' 'done';`},
	{"an if that is the whole branch", `
  if $Flag then
    if $Other then
      log info node 'M' 'b';
    else
      log info node 'M' 'c';
    end if;
  else
    log info node 'M' 'd';
  end if;
  log info node 'M' 'done';`},
	{"an if followed by an activity in a branch", `
  if $Flag then
    if $Other then
      log info node 'M' 'b';
    else
      log info node 'M' 'c';
    end if;
    log info node 'M' 'x';
  else
    log info node 'M' 'd';
  end if;
  log info node 'M' 'done';`},
	{"three ifs closing together", `
  if $Flag then
    if $Other then
      if $Flag then
        log info node 'M' 'b';
      end if;
    end if;
  end if;
  log info node 'M' 'done';`},
	{"a loop ending in an if, inside an if", `
  if $Flag then
    loop $It in $Items begin
      if $Other then
        log info node 'M' 'b';
      end if;
    end loop;
  end if;
  log info node 'M' 'done';`},
	{"nested loops with merges", `
  loop $It in $Items begin
    if $Flag then
      log info node 'M' 'a';
    end if;
    loop $Inner in $Items begin
      if $Other then
        log info node 'M' 'b';
      else
        log info node 'M' 'c';
      end if;
      log info node 'M' 'd';
    end loop;
    log info node 'M' 'e';
  end loop;
  log info node 'M' 'done';`},
}

// loopEscapes lists every object that is not wholly inside the loop that holds
// it, each loop in its own frame: what lint's MPR011 reports.
func loopEscapes(objects []microflows.MicroflowObject) []string {
	var out []string
	for _, o := range objects {
		loop, ok := o.(*microflows.LoopedActivity)
		if !ok || loop.ObjectCollection == nil {
			continue
		}
		for _, child := range loop.ObjectCollection.Objects {
			if _, note := child.(*microflows.Annotation); note {
				continue
			}
			b := canvasBoxOf(child)
			if b.left < 0 || b.top < 0 || b.right > loop.Size.Width || b.bottom > loop.Size.Height {
				out = append(out, fmt.Sprintf("%s outside its loop (%dx%d)", canvasLabel(child), loop.Size.Width, loop.Size.Height))
			}
		}
		out = append(out, loopEscapes(loop.ObjectCollection.Objects)...)
	}
	return out
}

// The guard the issue proposes — a layout must not add MPR008 or MPR011 findings
// — at the layer the positions are computed in: every shape, everywhere the
// builder puts a run, has no two objects sharing a place (MPR008, which measures
// each node by its stored size) and no object outside its loop (MPR011).
func TestConsecutiveMergesNeverOverlap(t *testing.T) {
	for _, shape := range consecutiveMergeShapes {
		for _, ctx := range allShapeContexts {
			t.Run(shape.name+" / "+ctx.name, func(t *testing.T) {
				src := handlerShapeFlow(ctx.wrap(shape.body))
				assertNoSharedPlace(t, src, true)
				if escapes := loopEscapes(buildLayout(t, src).Objects); len(escapes) > 0 {
					t.Errorf("objects outside their loop:\n  %s", strings.Join(escapes, "\n  "))
				}
			})
		}
	}
}

func TestConsecutiveMergesAreMeasuredAsBuilt(t *testing.T) {
	for _, shape := range consecutiveMergeShapes {
		t.Run(shape.name, func(t *testing.T) { assertLoopMeasuredAsBuilt(t, shape.body) })
	}
}
