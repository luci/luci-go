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
	"errors"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

func (p *processor) PrepareMutation(ctx context.Context, events Events, s State) (ts []Transition, _ Events, err error) {
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
					return Emit(ctx, []byte{'-'}, mkRecipient(ctx, p.index+1))
				},
				Events:       nil,        // Don't consume any events.
				TransitionTo: population, // Same state.
			})
		case *population < 3:
			population = add(+3)
			logging.Debugf(ctx, "growing +3=> %d", *population)
			ts = append(ts, Transition{
				SideEffectFn: nil,
				Events:       nil, // Don't consume any events.
				TransitionTo: population,
			})
		}
		return
	}

	// Triage events.
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
			Events: minus, // Always consume all advertisements to emigrate.
		}
		if *population <= 1 {
			logging.Debugf(ctx, "consuming %d ads", len(minus))
		} else {
			population = add(-1)
			t.SideEffectFn = func(ctx context.Context) error {
				logging.Debugf(ctx, "emigrated to %d", p.index-1)
				return Emit(ctx, []byte{'+'}, mkRecipient(ctx, p.index-1))
			}
		}
		t.TransitionTo = population
		ts = append(ts, t)
	}
	return
}

func mkRecipient(ctx context.Context, id int) Recipient {
	return Recipient{
		Key:              datastore.MakeKey(ctx, "cell", id),
		MonitoringString: fmt.Sprintf("cell-%d", id),
	}
}

func TestEventboxWorks(t *testing.T) {
	t.Parallel()

	Convey("eventbox works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const limit = 10000

		// Seed the first cell.
		So(Emit(ctx, []byte{'+'}, mkRecipient(ctx, 65)), ShouldBeNil)
		l, err := List(ctx, mkRecipient(ctx, 65))
		So(err, ShouldBeNil)
		So(l, ShouldHaveLength, 1)

		ppfns, err := ProcessBatch(ctx, mkRecipient(ctx, 65), &processor{65}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustGet(ctx, 65).EVersion, ShouldEqual, 1)
		So(mustGet(ctx, 65).Population, ShouldEqual, 1)
		So(mustList(ctx, 65), ShouldHaveLength, 0)

		// Let the cell grow without incoming events.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 65), &processor{65}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustGet(ctx, 65).EVersion, ShouldEqual, 2)
		So(mustGet(ctx, 65).Population, ShouldEqual, 1+3)
		// Can't grow any more, no change to anything.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 65), &processor{65}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustGet(ctx, 65).EVersion, ShouldEqual, 2)
		So(mustGet(ctx, 65).Population, ShouldEqual, 1+3)

		// Advertise from nearby cell, twice.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 64), &processor{64}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 64), &processor{64}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustList(ctx, 65), ShouldHaveLength, 2)
		// Emigrate, at most once.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 65), &processor{65}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustGet(ctx, 65).EVersion, ShouldEqual, 3)
		So(mustGet(ctx, 65).Population, ShouldEqual, 4-1)
		So(mustList(ctx, 65), ShouldHaveLength, 0)

		// Accept immigrants.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 64), &processor{64}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustGet(ctx, 64).Population, ShouldEqual, +1)

		// Advertise to a cell with population = 1 is a noop.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 63), &processor{63}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 64), &processor{64}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)

		// Lots of events at once.
		So(Emit(ctx, []byte{'+'}, mkRecipient(ctx, 49)), ShouldBeNil)
		So(Emit(ctx, []byte{'+'}, mkRecipient(ctx, 49)), ShouldBeNil) // will have to wait
		So(Emit(ctx, []byte{'+'}, mkRecipient(ctx, 49)), ShouldBeNil) // will have to wait
		So(Emit(ctx, []byte{'-'}, mkRecipient(ctx, 49)), ShouldBeNil) // not enough people, ignored.
		So(Emit(ctx, []byte{'-'}, mkRecipient(ctx, 49)), ShouldBeNil) // not enough people, ignored.
		So(mustList(ctx, 49), ShouldHaveLength, 5)
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 49), &processor{49}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustGet(ctx, 49).EVersion, ShouldEqual, 1)
		So(mustGet(ctx, 49).Population, ShouldEqual, 1)
		So(mustList(ctx, 49), ShouldHaveLength, 2) // 2x'+' are waiting
		// Slowly welcome remaining newcomers.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 49), &processor{49}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustGet(ctx, 49).Population, ShouldEqual, 2)
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 49), &processor{49}, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustGet(ctx, 49).Population, ShouldEqual, 3)
		// Finally, must be done.
		So(mustList(ctx, 49), ShouldHaveLength, 0)
	})
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
	l, err := List(ctx, mkRecipient(ctx, index))
	So(err, ShouldBeNil)
	return l
}

func TestEventboxPostProcessFn(t *testing.T) {
	t.Parallel()

	Convey("eventbox", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const limit = 10000
		recipient := mkRecipient(ctx, 753)

		initState := int(149)
		p := &mockProc{
			loadState: func(context.Context) (State, EVersion, error) {
				return State(&initState), EVersion(0), nil
			},
			fetchEVersion: func(context.Context) (EVersion, error) {
				return 0, nil
			},
			saveState: func(context.Context, State, EVersion) error {
				return nil
			},
		}
		Convey("Returns PostProcessFns for successful state transitions", func() {
			var calledForStates []int
			p.prepareMutation = func(ctx context.Context, es Events, s State) ([]Transition, Events, error) {
				gotState := *(s.(*int))
				return []Transition{
					{
						TransitionTo: gotState + 1,
						PostProcessFn: func(ctx context.Context) error {
							calledForStates = append(calledForStates, gotState+1)
							return nil
						},
					},
					{
						TransitionTo: gotState + 2,
					},
					{
						TransitionTo: gotState + 3,
						PostProcessFn: func(ctx context.Context) error {
							calledForStates = append(calledForStates, gotState+3)
							return nil
						},
					},
				}, nil, nil
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldBeNil)
			So(ppfns, ShouldHaveLength, 2)
			for _, ppfn := range ppfns {
				So(ppfn(ctx), ShouldBeNil)
			}
			So(calledForStates, ShouldResemble, []int{150, 152})
		})
	})
}

func TestEventboxFails(t *testing.T) {
	t.Parallel()

	Convey("eventbox fails as intended in failure cases", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const limit = 100000
		recipient := mkRecipient(ctx, 77)

		So(Emit(ctx, []byte{'+'}, recipient), ShouldBeNil)
		So(Emit(ctx, []byte{'-'}, recipient), ShouldBeNil)

		initState := int(99)
		p := &mockProc{
			loadState: func(_ context.Context) (State, EVersion, error) {
				return State(&initState), EVersion(0), nil
			},
			// since 3 other funcs are nil, calling their Upper-case counterparts will
			// panic (see mockProc implementation below).
		}
		Convey("Mutate() failure aborts", func() {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return nil, nil, errors.New("oops")
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldErrLike, "oops")
			So(ppfns, ShouldBeEmpty)
		})

		firstSideEffectCalled := false
		const firstIndex = 88
		secondState := initState + 1
		var second SideEffectFn
		p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
			return []Transition{
				{
					SideEffectFn: func(ctx context.Context) error {
						firstSideEffectCalled = true
						return datastore.Put(ctx, &cell{Index: firstIndex})
					},
					Events:       es[:1],
					TransitionTo: s,
				},
				{
					SideEffectFn: second,
					Events:       es[1:],
					TransitionTo: State(&secondState),
				},
			}, nil, nil
		}

		Convey("Eversion must be checked", func() {
			p.fetchEVersion = func(_ context.Context) (EVersion, error) {
				return 0, errors.New("ev error")
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldErrLike, "ev error")
			So(ppfns, ShouldBeEmpty)
			p.fetchEVersion = func(_ context.Context) (EVersion, error) {
				return 1, nil
			}
			ppfns, err = ProcessBatch(ctx, recipient, p, limit)
			So(common.DSContentionTag.In(err), ShouldBeTrue)
			So(ppfns, ShouldBeEmpty)
			So(firstSideEffectCalled, ShouldBeFalse)
		})

		p.fetchEVersion = func(_ context.Context) (EVersion, error) {
			return 0, nil
		}

		Convey("No call to save if any Transition fails", func() {
			second = func(_ context.Context) error {
				return transient.Tag.Apply(errors.New("2nd failed"))
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldErrLike, "2nd failed")
			So(ppfns, ShouldBeEmpty)
			So(firstSideEffectCalled, ShouldBeTrue)
			// ... but w/o any effect since transaction should have been aborted
			So(datastore.Get(ctx, &cell{Index: firstIndex}),
				ShouldEqual, datastore.ErrNoSuchEntity)
		})

		second = func(_ context.Context) error { return nil }
		Convey("Failed Save aborts any side effects, too", func() {
			p.saveState = func(ctx context.Context, st State, ev EVersion) error {
				s := *(st.(*int))
				So(ev, ShouldEqual, 1)
				So(s, ShouldNotEqual, initState)
				So(s, ShouldEqual, secondState)
				return transient.Tag.Apply(errors.New("savvvvvvvvvvvvvvvvvvvvvvvvvv hung"))
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldErrLike, "savvvvvvvvvvvvvvvv")
			So(ppfns, ShouldBeEmpty)
			// ... still no side effect.
			So(datastore.Get(ctx, &cell{Index: firstIndex}),
				ShouldEqual, datastore.ErrNoSuchEntity)
		})

		// In all cases, there must still be 2 unconsumed events.
		l, err := List(ctx, recipient)
		So(err, ShouldBeNil)
		So(l, ShouldHaveLength, 2)

		// Finally, check that first side effect is real, otherwise assertions above
		// might be giving false sense of correctness.
		p.saveState = func(context.Context, State, EVersion) error { return nil }
		ppfns, err := ProcessBatch(ctx, recipient, p, limit)
		So(err, ShouldBeNil)
		So(ppfns, ShouldBeEmpty)
		So(mustGet(ctx, firstIndex), ShouldNotBeNil)
	})
}

func TestEventboxNoopTransitions(t *testing.T) {
	t.Parallel()

	Convey("Noop Transitions are detected", t, func() {
		t := Transition{}
		So(t.isNoop(nil), ShouldBeTrue)
		initState := int(99)
		t.TransitionTo = initState
		So(t.isNoop(nil), ShouldBeFalse)
		So(t.isNoop(initState), ShouldBeTrue)
		t.Events = Events{Event{}}
		So(t.isNoop(initState), ShouldBeFalse)
		t.Events = nil
		t.SideEffectFn = func(context.Context) error { return nil }
		So(t.isNoop(initState), ShouldBeFalse)
		t.SideEffectFn = nil
		t.PostProcessFn = func(context.Context) error { return nil }
		So(t.isNoop(initState), ShouldBeFalse)
	})

	Convey("eventbox doesn't transact on nil transitions", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const limit = 100000
		recipient := mkRecipient(ctx, 77)
		initState := int(99)
		panicErr := errors.New("must not be transact!")

		p := &mockProc{
			loadState: func(_ context.Context) (State, EVersion, error) {
				return State(&initState), EVersion(0), nil
			},
			fetchEVersion: func(_ context.Context) (EVersion, error) {
				panic(panicErr)
			},
		}

		Convey("Mutate returns no transitions", func() {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return nil, nil, nil
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldBeNil)
			So(ppfns, ShouldBeEmpty)
		})
		Convey("Mutate returns no transitions, but some semantic garbage is still cleaned up", func() {
			So(Emit(ctx, []byte("msg"), recipient), ShouldBeNil)
			So(Emit(ctx, []byte("msg"), recipient), ShouldBeNil)
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return nil, es[:1], nil
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldBeNil)
			So(ppfns, ShouldBeEmpty)
			l, err := List(ctx, recipient)
			So(err, ShouldBeNil)
			So(l, ShouldHaveLength, 1)
			So(l[0].Value, ShouldResemble, []byte("msg"))
		})
		Convey("Garbage is cleaned up even if Mutate also returns error", func() {
			So(Emit(ctx, []byte("msg"), recipient), ShouldBeNil)
			So(Emit(ctx, []byte("msg"), recipient), ShouldBeNil)
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return nil, es[:1], errors.New("boom")
			}
			_, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldErrLike, "boom")
			l, err := List(ctx, recipient)
			So(err, ShouldBeNil)
			So(l, ShouldHaveLength, 1)
			So(l[0].Value, ShouldResemble, []byte("msg"))
		})
		Convey("Mutate returns empty slice of transitions", func() {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return []Transition{}, nil, nil
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldBeNil)
			So(ppfns, ShouldBeEmpty)
		})
		Convey("Mutate returns noop transitions only", func() {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return []Transition{
					{TransitionTo: s},
				}, nil, nil
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			So(err, ShouldBeNil)
			So(ppfns, ShouldBeEmpty)
		})

		Convey("Test's own sanity check that fetchEVersion is called and panics", func() {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return []Transition{
					{TransitionTo: new(int)},
				}, nil, nil
			}
			So(func() { ProcessBatch(ctx, recipient, p, limit) }, ShouldPanicLike, panicErr)
		})
	})
}

type mockProc struct {
	loadState       func(_ context.Context) (State, EVersion, error)
	prepareMutation func(_ context.Context, _ Events, _ State) ([]Transition, Events, error)
	fetchEVersion   func(_ context.Context) (EVersion, error)
	saveState       func(_ context.Context, _ State, _ EVersion) error
}

func (m *mockProc) LoadState(ctx context.Context) (State, EVersion, error) {
	return m.loadState(ctx)
}
func (m *mockProc) PrepareMutation(ctx context.Context, e Events, s State) ([]Transition, Events, error) {
	return m.prepareMutation(ctx, e, s)
}
func (m *mockProc) FetchEVersion(ctx context.Context) (EVersion, error) {
	return m.fetchEVersion(ctx)
}
func (m *mockProc) SaveState(ctx context.Context, s State, e EVersion) error {
	return m.saveState(ctx, s, e)
}

func PrepareMutation(t *testing.T) {
	t.Parallel()

	Convey("Chain of SideEffectFn works", t, func() {
		ctx := context.Background()
		var ops []string
		f1 := func(context.Context) error {
			ops = append(ops, "f1")
			return nil
		}
		f2 := func(context.Context) error {
			ops = append(ops, "f2")
			return nil
		}
		breakChain := errors.New("break")
		ferr := func(context.Context) error {
			ops = append(ops, "ferr")
			return breakChain
		}
		Convey("all nils chain to nil", func() {
			So(Chain(), ShouldBeNil)
			So(Chain(nil), ShouldBeNil)
			So(Chain(nil, nil), ShouldBeNil)
		})
		Convey("order is respected", func() {
			So(Chain(nil, f2, nil, f1, f2, f1, nil)(ctx), ShouldBeNil)
			So(ops, ShouldResemble, []string{"f2", "f1", "f2", "f1"})
		})
		Convey("error aborts", func() {
			So(Chain(f1, nil, ferr, f2)(ctx), ShouldErrLike, breakChain)
			So(ops, ShouldResemble, []string{"f1", "ferr"})
		})
	})
}
