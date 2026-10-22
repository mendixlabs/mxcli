package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A fall-through custom error handler on an activity inside a branch continues
// with the statement after the failing activity, as it does at the top level
// (mendixlabs/mxcli#1565). It rejoined on the flow INTO the split instead — a
// retry loop for as long as the call kept failing — when the call assigned a
// variable, and ended the microflow at an end event of its own when it did not.
func TestBranchErrorHandlerRejoinsAfterTheFailingActivity(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// rejoin is the log the handler's tail must reach through merges only.
		rejoin string
		ends   int
	}{
		{
			name: "if without else, call with a result",
			src: `create microflow M.A ($Flag: Boolean)
begin
  log info node 'P' 'before';
  if $Flag then
    $N = call microflow M.Sub() on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';
end;`,
			rejoin: "after call",
			ends:   1,
		},
		{
			name: "if without else, call without a result",
			src: `create microflow M.B ($Flag: Boolean)
begin
  log info node 'P' 'before';
  if $Flag then
    call microflow M.Sub() on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';
end;`,
			rejoin: "after call",
			ends:   1,
		},
		{
			name: "then branch that returns",
			src: `create microflow M.C ($Flag: Boolean) returns Boolean
begin
  if $Flag then
    call microflow M.Sub() on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
    return true;
  else
    log info node 'P' 'else';
  end if;
  log info node 'P' 'end';
  return false;
end;`,
			rejoin: "after call",
			ends:   2,
		},
		{
			name: "else branch",
			src: `create microflow M.D ($Flag: Boolean) returns Boolean
begin
  if $Flag then
    return true;
  else
    call microflow M.Sub() on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';
  return false;
end;`,
			rejoin: "after call",
			ends:   2,
		},
		{
			name: "handler on the branch's last activity",
			src: `create microflow M.E ($Flag: Boolean)
begin
  if $Flag then
    log info node 'P' 'first';
    $N = call microflow M.Sub() on error without rollback begin
      log warning node 'P' 'failed';
    end error;
  end if;
  log info node 'P' 'end';
end;`,
			rejoin: "end",
			ends:   1,
		},
		{
			name: "result used by the next branch statement",
			src: `create microflow M.F ($Flag: Boolean)
begin
  if $Flag then
    $N = call microflow M.Sub() on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'n=' + toString($N);
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';
end;`,
			rejoin: "after call",
			ends:   1,
		},
		{
			// The rest of the branch builds on the result, so the handler skips
			// it for the code after the IF, the branch's shared error path
			// (TestBuildFlowGraph_OutputHandlerInReturningBranchSkipsDerivedSuccessTail),
			// on the flow leaving the IF, not the one into it.
			name: "result used by the rest of a branch that returns",
			src: `create microflow M.K ($Flag: Boolean)
begin
  if $Flag then
    $N = call microflow M.Sub() on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'n=' + toString($N);
    return;
  end if;
  log info node 'P' 'end';
end;`,
			rejoin: "end",
			ends:   2,
		},
		{
			name: "nested if",
			src: `create microflow M.G ($Flag: Boolean, $Other: Boolean)
begin
  if $Flag then
    if $Other then
      call microflow M.Sub() on error without rollback begin
        log warning node 'P' 'failed';
      end error;
      log info node 'P' 'after call';
    end if;
    log info node 'P' 'outer';
  end if;
  log info node 'P' 'end';
end;`,
			rejoin: "after call",
			ends:   1,
		},
		{
			name: "case branch",
			src: `create microflow M.H ($C: M.Color)
begin
  case $C
    when Red then
      call microflow M.Sub() on error without rollback begin
        log warning node 'P' 'failed';
      end error;
      log info node 'P' 'after call';
    when Green, (empty) then
      log info node 'P' 'green';
  end case;
  log info node 'P' 'end';
end;`,
			rejoin: "after call",
			ends:   1,
		},
		{
			name: "split type branch",
			src: `create microflow M.I ($In: M.Base)
begin
  split type $In
  when M.Special then
    call microflow M.Sub() on error without rollback begin
      log warning node 'P' 'failed';
    end error;
  when M.Base then
    log info node 'P' 'base';
  end split;
  log info node 'P' 'end';
end;`,
			rejoin: "end",
			ends:   1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oc := buildMicroflowFromMDL(t, tc.src)
			logs := logActivitiesByText(oc)
			failed, want := logs["failed"], logs[tc.rejoin]
			if failed == "" || want == "" {
				t.Fatalf("missing log activities in %v", logs)
			}
			if got := throughMerges(oc, failed); got != want {
				t.Errorf("handler rejoins at %s, want the %q log %s", describeObject(oc, got), tc.rejoin, want)
			}
			if n := countEndEvents(oc); n != tc.ends {
				t.Errorf("got %d end events, want %d", n, tc.ends)
			}
			if hasCycle(oc) {
				t.Error("the flow graph has a cycle")
			}
		})
	}
}

// A handler still pending when an IF starts belongs to the activity before
// it: the branches must leave it alone, so it rejoins on the flow into the
// split, as before.
func TestErrorHandlerBeforeAnIfStillRejoinsAtTheSplit(t *testing.T) {
	oc := buildMicroflowFromMDL(t, `create microflow M.J ($Flag: Boolean)
begin
  $N = call microflow M.Sub() on error without rollback begin
    log warning node 'P' 'failed';
  end error;
  if $Flag then
    log info node 'P' 'a';
    log info node 'P' 'b';
  end if;
  log info node 'P' 'end';
end;`)
	got := throughMerges(oc, logActivitiesByText(oc)["failed"])
	if _, ok := objectByID(oc.Objects, got).(*microflows.ExclusiveSplit); !ok {
		t.Errorf("handler rejoins at %s, want the IF's split", describeObject(oc, got))
	}
	if hasCycle(oc) {
		t.Error("the flow graph has a cycle")
	}
}

func logActivitiesByText(oc *microflows.MicroflowObjectCollection) map[string]model.ID {
	ids := map[string]model.ID{}
	for _, obj := range oc.Objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		if action, ok := activity.Action.(*microflows.LogMessageAction); ok && action.MessageTemplate != nil {
			for _, text := range action.MessageTemplate.Translations {
				ids[text] = activity.ID
			}
		}
	}
	return ids
}

// throughMerges follows the single outgoing flow from id past any merges and
// returns the first object that is not one.
func throughMerges(oc *microflows.MicroflowObjectCollection, id model.ID) model.ID {
	for range len(oc.Objects) {
		var next model.ID
		for _, f := range oc.Flows {
			if f.OriginID == id {
				next = f.DestinationID
				break
			}
		}
		if next == "" {
			return ""
		}
		if _, ok := objectByID(oc.Objects, next).(*microflows.ExclusiveMerge); !ok {
			return next
		}
		id = next
	}
	return ""
}

func describeObject(oc *microflows.MicroflowObjectCollection, id model.ID) string {
	if id == "" {
		return "nothing"
	}
	for text, logID := range logActivitiesByText(oc) {
		if logID == id {
			return "the " + text + " log"
		}
	}
	switch objectByID(oc.Objects, id).(type) {
	case *microflows.EndEvent:
		return "an end event"
	case *microflows.ExclusiveSplit:
		return "a split"
	}
	return string(id)
}

func countEndEvents(oc *microflows.MicroflowObjectCollection) int {
	n := 0
	for _, obj := range oc.Objects {
		if _, ok := obj.(*microflows.EndEvent); ok {
			n++
		}
	}
	return n
}

func hasCycle(oc *microflows.MicroflowObjectCollection) bool {
	next := map[model.ID][]model.ID{}
	for _, f := range oc.Flows {
		next[f.OriginID] = append(next[f.OriginID], f.DestinationID)
	}
	const (
		unseen = iota
		open
		done
	)
	state := map[model.ID]int{}
	var visit func(model.ID) bool
	visit = func(id model.ID) bool {
		switch state[id] {
		case open:
			return true
		case done:
			return false
		}
		state[id] = open
		for _, to := range next[id] {
			if visit(to) {
				return true
			}
		}
		state[id] = done
		return false
	}
	for id := range next {
		if visit(id) {
			return true
		}
	}
	return false
}
