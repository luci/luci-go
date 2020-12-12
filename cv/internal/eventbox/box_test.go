// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eventbox

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

// processor simulates a variant of game of life on one cell in an array of
// cells.
type processor struct {
	index int
}

type cell struct {
	Index      int      `gae:"$id"`
	EVersion   EVersion `gae:",noindex"`
	Population int      `gae:",noindex"`
}

func (p *processor) LoadState(ctx context.Context) (State, EVersion, error) {
	c, err := get(ctx, p.index)
	if err != nil {
		return nil, 0, err
	}
	return State(&c.Population), c.EVersion, nil
}

func (p *processor) FetchEVersion(ctx context.Context) (EVersion, error) {
	c, err := get(ctx, p.index)
	if err != nil {
		return 0, err
	}
	return c.EVersion, nil
}

func (p *processor) SaveState(ctx context.Context, s State, e EVersion) error {
	c := cell{Index: p.index, EVersion: e, Population: *(s.(*int))}
	return transient.Tag.Apply(datastore.Put(ctx, &c))
}

func (p *processor) Mutate(ctx context.Context, events Events, s State) (ts []Transition, err error) {
	ctx = logging.SetField(ctx, "index", p.index)
	// Simulate variation of game of life.
	population := s.(*int)
	add := func(delta int) *int {
		n := new(int)
		*n = delta + (*population)
		return n
	}

	if len(events) == 0 {
		switch {
		case *population == 0:
			ts = append(ts, Transition{
				SideEffectFn: func(ctx context.Context) error {
					logging.Debugf(ctx, "advertised to %d to migrate", p.index+1)
					return Emit(ctx, []byte{'-'}, key(ctx, p.index+1))
				},
				Events:       nil,        // don't consume any events
				TransitionTo: population, // same state
			})
		case *population < 3:
			population = add(+3)
			logging.Debugf(ctx, "growing +3=> %d", *population)
			ts = append(ts, Transition{
				SideEffectFn: nil,
				Events:       nil, // don't consume any events
				TransitionTo: population,
			})
		}
		return
	}

	// triage events
	var minus, plus Events
	for _, e := range events {
		if e.Value[0] == '-' {
			minus = append(minus, e)
		} else {
			plus = append(plus, e)
		}
	}

	if len(plus) > 0 {
		// Accept at most 1 at a time.
		population = add(1)
		logging.Debugf(ctx, "welcoming +1 out of %d => %d", len(plus), *population)
		ts = append(ts, Transition{
			SideEffectFn: nil,
			Events:       plus[:1], // Consume only 1 event.
			TransitionTo: population,
		})
	}
	if len(minus) > 0 {
		t := Transition{
			Events: minus, // always consume all advertisments to emmigrate.
		}
		if *population <= 1 {
			logging.Debugf(ctx, "consuming %d ads", len(minus))
		} else {
			population = add(-1)
			t.SideEffectFn = func(ctx context.Context) error {
				logging.Debugf(ctx, "emigrated to %d", p.index-1)
				return Emit(ctx, []byte{'+'}, key(ctx, p.index-1))
			}
		}
		t.TransitionTo = population
		ts = append(ts, t)
	}
	return
}

func TestEventbox(t *testing.T) {
	t.Parallel()

	Convey("eventbox works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// Seed the first cell.
		So(Emit(ctx, []byte{'+'}, key(ctx, 65)), ShouldBeNil)
		l, err := List(ctx, key(ctx, 65))
		So(err, ShouldBeNil)
		So(l, ShouldHaveLength, 1)

		So(ProcessBatch(ctx, key(ctx, 65), &processor{65}), ShouldBeNil)
		So(mustGet(ctx, 65).EVersion, ShouldEqual, 1)
		So(mustGet(ctx, 65).Population, ShouldEqual, 1)
		So(mustList(ctx, 65), ShouldHaveLength, 0)

		// Let the cell grow without incoming events.
		So(ProcessBatch(ctx, key(ctx, 65), &processor{65}), ShouldBeNil)
		So(mustGet(ctx, 65).EVersion, ShouldEqual, 2)
		So(mustGet(ctx, 65).Population, ShouldEqual, 1+3)
		// Can't grow any more, no change to anything.
		So(ProcessBatch(ctx, key(ctx, 65), &processor{65}), ShouldBeNil)
		So(mustGet(ctx, 65).EVersion, ShouldEqual, 2)
		So(mustGet(ctx, 65).Population, ShouldEqual, 1+3)

		// Advertise from nearby cell, twice.
		So(ProcessBatch(ctx, key(ctx, 64), &processor{64}), ShouldBeNil)
		So(ProcessBatch(ctx, key(ctx, 64), &processor{64}), ShouldBeNil)
		So(mustList(ctx, 65), ShouldHaveLength, 2)
		// Emigrate, at most once.
		So(ProcessBatch(ctx, key(ctx, 65), &processor{65}), ShouldBeNil)
		So(mustGet(ctx, 65).EVersion, ShouldEqual, 3)
		So(mustGet(ctx, 65).Population, ShouldEqual, 4-1)
		So(mustList(ctx, 65), ShouldHaveLength, 0)

		// Accept immigrants.
		So(ProcessBatch(ctx, key(ctx, 64), &processor{64}), ShouldBeNil)
		So(mustGet(ctx, 64).Population, ShouldEqual, +1)

		// Advertise to a cell with population = 1 is a noop.
		So(ProcessBatch(ctx, key(ctx, 63), &processor{63}), ShouldBeNil)
		So(ProcessBatch(ctx, key(ctx, 64), &processor{64}), ShouldBeNil)

		// Lots of events at once.
		So(Emit(ctx, []byte{'+'}, key(ctx, 49)), ShouldBeNil)
		So(Emit(ctx, []byte{'+'}, key(ctx, 49)), ShouldBeNil) // will have to wait
		So(Emit(ctx, []byte{'+'}, key(ctx, 49)), ShouldBeNil) // will have to wait
		So(Emit(ctx, []byte{'-'}, key(ctx, 49)), ShouldBeNil) // not enough people, ignored.
		So(Emit(ctx, []byte{'-'}, key(ctx, 49)), ShouldBeNil) // not enough people, ignored.
		So(mustList(ctx, 49), ShouldHaveLength, 5)
		So(ProcessBatch(ctx, key(ctx, 49), &processor{49}), ShouldBeNil)
		So(mustGet(ctx, 49).EVersion, ShouldEqual, 1)
		So(mustGet(ctx, 49).Population, ShouldEqual, 1)
		So(mustList(ctx, 49), ShouldHaveLength, 2) // 2x'+' are waiting
		// Slowly welcome remaining newcomers.
		So(ProcessBatch(ctx, key(ctx, 49), &processor{49}), ShouldBeNil)
		So(mustGet(ctx, 49).Population, ShouldEqual, 2)
		So(ProcessBatch(ctx, key(ctx, 49), &processor{49}), ShouldBeNil)
		So(mustGet(ctx, 49).Population, ShouldEqual, 3)
		// Finally, must be done.
		So(mustList(ctx, 49), ShouldHaveLength, 0)
	})
}

func key(ctx context.Context, id int) *datastore.Key {
	return datastore.MakeKey(ctx, "cell", id)
}

func get(ctx context.Context, index int) (*cell, error) {
	c := &cell{Index: index}
	switch err := datastore.Get(ctx, c); {
	case err == datastore.ErrNoSuchEntity || err == nil:
		return c, nil
	default:
		return nil, transient.Tag.Apply(err)
	}
}

func mustGet(ctx context.Context, index int) *cell {
	c, err := get(ctx, index)
	So(err, ShouldBeNil)
	return c
}

func mustList(ctx context.Context, index int) Events {
	l, err := List(ctx, key(ctx, index))
	So(err, ShouldBeNil)
	return l
}
