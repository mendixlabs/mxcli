// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	genSched "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/scheduledevents"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// Codec-native scheduled-event read. Used by SHOW STRUCTURE (per-module counts)
// and the project tree; the modelsdk engine previously left these unimplemented,
// so scheduled events were silently undercounted (the callers swallow the
// not-implemented error). Unlike JavaScript actions, the gen codec decodes
// ScheduledEvent under the same storage keys the legacy parser uses (Name /
// Documentation / Microflow / Enabled / Interval / IntervalType), so gen
// accessors are correct here — no raw-key reads needed.

func (b *Backend) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	units, err := mprread.ListUnitsWithContainer[*genSched.ScheduledEvent](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ScheduledEvent, 0, len(units))
	for _, u := range units {
		out = append(out, scheduledEventFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

func (b *Backend) GetScheduledEvent(id model.ID) (*model.ScheduledEvent, error) {
	units, err := mprread.ListUnitsWithContainer[*genSched.ScheduledEvent](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return scheduledEventFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, fmt.Errorf("scheduled event not found: %s", id)
}

// scheduledEventFromGen converts a gen ScheduledEvent to the semantic type. Mirrors
// the legacy parseScheduledEvent field set: MicroflowID holds the by-name microflow
// reference (BSON "Microflow"), and Interval comes through the int32 decoder (which
// accepts the int64 Studio Pro actually writes — issue #585).
func scheduledEventFromGen(g *genSched.ScheduledEvent, containerID model.ID) *model.ScheduledEvent {
	ev := &model.ScheduledEvent{
		ContainerID:   containerID,
		Name:          g.Name(),
		Documentation: g.Documentation(),
		MicroflowID:   model.ID(g.MicroflowQualifiedName()),
		Interval:      int(g.Interval()),
		IntervalType:  g.IntervalType(),
		Enabled:       g.Enabled(),
	}
	ev.ID = model.ID(g.ID())
	ev.TypeName = "ScheduledEvents$ScheduledEvent"
	return ev
}
