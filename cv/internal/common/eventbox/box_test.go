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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
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
		tsn := Transition{
			Events: minus, // Always consume all advertisements to emigrate.
		}
		if *population <= 1 {
			logging.Debugf(ctx, "consuming %d ads", len(minus))
		} else {
			population = add(-1)
			tsn.SideEffectFn = func(ctx context.Context) error {
				logging.Debugf(ctx, "emigrated to %d", p.index-1)
				return Emit(ctx, []byte{'+'}, mkRecipient(ctx, p.index-1))
			}
		}
		tsn.TransitionTo = population
		ts = append(ts, tsn)
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

	ftt.Run("eventbox works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const limit = 10000

		// Seed the first cell.
		assert.Loosely(t, Emit(ctx, []byte{'+'}, mkRecipient(ctx, 65)), should.BeNil)
		l, err := List(ctx, mkRecipient(ctx, 65))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, l, should.HaveLength(1))

		ppfns, err := ProcessBatch(ctx, mkRecipient(ctx, 65), &processor{65}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustGet(t, ctx, 65).EVersion, should.Equal(1))
		assert.Loosely(t, mustGet(t, ctx, 65).Population, should.Equal(1))
		assert.Loosely(t, mustList(t, ctx, 65), should.HaveLength(0))

		// Let the cell grow without incoming events.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 65), &processor{65}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustGet(t, ctx, 65).EVersion, should.Equal(2))
		assert.Loosely(t, mustGet(t, ctx, 65).Population, should.Equal(1+3))
		// Can't grow any more, no change to anything.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 65), &processor{65}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustGet(t, ctx, 65).EVersion, should.Equal(2))
		assert.Loosely(t, mustGet(t, ctx, 65).Population, should.Equal(1+3))

		// Advertise from nearby cell, twice.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 64), &processor{64}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 64), &processor{64}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustList(t, ctx, 65), should.HaveLength(2))
		// Emigrate, at most once.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 65), &processor{65}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustGet(t, ctx, 65).EVersion, should.Equal(3))
		assert.Loosely(t, mustGet(t, ctx, 65).Population, should.Equal(4-1))
		assert.Loosely(t, mustList(t, ctx, 65), should.HaveLength(0))

		// Accept immigrants.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 64), &processor{64}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustGet(t, ctx, 64).Population, should.Equal(+1))

		// Advertise to a cell with population = 1 is a noop.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 63), &processor{63}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 64), &processor{64}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)

		// Lots of events at once.
		assert.Loosely(t, Emit(ctx, []byte{'+'}, mkRecipient(ctx, 49)), should.BeNil)
		assert.Loosely(t, Emit(ctx, []byte{'+'}, mkRecipient(ctx, 49)), should.BeNil) // will have to wait
		assert.Loosely(t, Emit(ctx, []byte{'+'}, mkRecipient(ctx, 49)), should.BeNil) // will have to wait
		assert.Loosely(t, Emit(ctx, []byte{'-'}, mkRecipient(ctx, 49)), should.BeNil) // not enough people, ignored.
		assert.Loosely(t, Emit(ctx, []byte{'-'}, mkRecipient(ctx, 49)), should.BeNil) // not enough people, ignored.
		assert.Loosely(t, mustList(t, ctx, 49), should.HaveLength(5))
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 49), &processor{49}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustGet(t, ctx, 49).EVersion, should.Equal(1))
		assert.Loosely(t, mustGet(t, ctx, 49).Population, should.Equal(1))
		assert.Loosely(t, mustList(t, ctx, 49), should.HaveLength(2)) // 2x'+' are waiting
		// Slowly welcome remaining newcomers.
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 49), &processor{49}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustGet(t, ctx, 49).Population, should.Equal(2))
		ppfns, err = ProcessBatch(ctx, mkRecipient(ctx, 49), &processor{49}, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustGet(t, ctx, 49).Population, should.Equal(3))
		// Finally, must be done.
		assert.Loosely(t, mustList(t, ctx, 49), should.HaveLength(0))
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

func mustGet(t testing.TB, ctx context.Context, index int) *cell {
	t.Helper()

	c, err := get(ctx, index)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return c
}

func mustList(t testing.TB, ctx context.Context, index int) Events {
	t.Helper()

	l, err := List(ctx, mkRecipient(ctx, index))
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return l
}

func TestEventboxPostProcessFn(t *testing.T) {
	t.Parallel()

	ftt.Run("eventbox", t, func(t *ftt.Test) {
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
		t.Run("Returns PostProcessFns for successful state transitions", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ppfns, should.HaveLength(2))
			for _, ppfn := range ppfns {
				assert.Loosely(t, ppfn(ctx), should.BeNil)
			}
			assert.Loosely(t, calledForStates, should.Resemble([]int{150, 152}))
		})
	})
}

func TestEventboxFails(t *testing.T) {
	t.Parallel()

	ftt.Run("eventbox fails as intended in failure cases", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const limit = 100000
		recipient := mkRecipient(ctx, 77)

		assert.Loosely(t, Emit(ctx, []byte{'+'}, recipient), should.BeNil)
		assert.Loosely(t, Emit(ctx, []byte{'-'}, recipient), should.BeNil)

		initState := int(99)
		p := &mockProc{
			loadState: func(_ context.Context) (State, EVersion, error) {
				return State(&initState), EVersion(0), nil
			},
			// since 3 other funcs are nil, calling their Upper-case counterparts will
			// panic (see mockProc implementation below).
		}
		t.Run("Mutate() failure aborts", func(t *ftt.Test) {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return nil, nil, errors.New("oops")
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, err, should.ErrLike("oops"))
			assert.Loosely(t, ppfns, should.BeEmpty)
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

		t.Run("Eversion must be checked", func(t *ftt.Test) {
			p.fetchEVersion = func(_ context.Context) (EVersion, error) {
				return 0, errors.New("ev error")
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, err, should.ErrLike("ev error"))
			assert.Loosely(t, ppfns, should.BeEmpty)
			p.fetchEVersion = func(_ context.Context) (EVersion, error) {
				return 1, nil
			}
			ppfns, err = ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, common.DSContentionTag.In(err), should.BeTrue)
			assert.Loosely(t, ppfns, should.BeEmpty)
			assert.Loosely(t, firstSideEffectCalled, should.BeFalse)
		})

		p.fetchEVersion = func(_ context.Context) (EVersion, error) {
			return 0, nil
		}

		t.Run("No call to save if any Transition fails", func(t *ftt.Test) {
			second = func(_ context.Context) error {
				return transient.Tag.Apply(errors.New("2nd failed"))
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, err, should.ErrLike("2nd failed"))
			assert.Loosely(t, ppfns, should.BeEmpty)
			assert.Loosely(t, firstSideEffectCalled, should.BeTrue)
			// ... but w/o any effect since transaction should have been aborted
			assert.Loosely(t, datastore.Get(ctx, &cell{Index: firstIndex}),
				should.Equal(datastore.ErrNoSuchEntity))
		})

		second = func(_ context.Context) error { return nil }
		t.Run("Failed Save aborts any side effects, too", func(t *ftt.Test) {
			p.saveState = func(ctx context.Context, st State, ev EVersion) error {
				s := *(st.(*int))
				assert.Loosely(t, ev, should.Equal(1))
				assert.Loosely(t, s, should.NotEqual(initState))
				assert.Loosely(t, s, should.Equal(secondState))
				return transient.Tag.Apply(errors.New("savvvvvvvvvvvvvvvvvvvvvvvvvv hung"))
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, err, should.ErrLike("savvvvvvvvvvvvvvvv"))
			assert.Loosely(t, ppfns, should.BeEmpty)
			// ... still no side effect.
			assert.Loosely(t, datastore.Get(ctx, &cell{Index: firstIndex}),
				should.Equal(datastore.ErrNoSuchEntity))
		})

		// In all cases, there must still be 2 unconsumed events.
		l, err := List(ctx, recipient)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, l, should.HaveLength(2))

		// Finally, check that first side effect is real, otherwise assertions above
		// might be giving false sense of correctness.
		p.saveState = func(context.Context, State, EVersion) error { return nil }
		ppfns, err := ProcessBatch(ctx, recipient, p, limit)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ppfns, should.BeEmpty)
		assert.Loosely(t, mustGet(t, ctx, firstIndex), should.NotBeNil)
	})
}

func TestEventboxNoopTransitions(t *testing.T) {
	t.Parallel()

	ftt.Run("Noop Transitions are detected", t, func(t *ftt.Test) {
		tsn := Transition{}
		assert.Loosely(t, tsn.isNoop(nil), should.BeTrue)
		initState := int(99)
		tsn.TransitionTo = initState
		assert.Loosely(t, tsn.isNoop(nil), should.BeFalse)
		assert.Loosely(t, tsn.isNoop(initState), should.BeTrue)
		tsn.Events = Events{Event{}}
		assert.Loosely(t, tsn.isNoop(initState), should.BeFalse)
		tsn.Events = nil
		tsn.SideEffectFn = func(context.Context) error { return nil }
		assert.Loosely(t, tsn.isNoop(initState), should.BeFalse)
		tsn.SideEffectFn = nil
		tsn.PostProcessFn = func(context.Context) error { return nil }
		assert.Loosely(t, tsn.isNoop(initState), should.BeFalse)
	})

	ftt.Run("eventbox doesn't transact on nil transitions", t, func(t *ftt.Test) {
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

		t.Run("Mutate returns no transitions", func(t *ftt.Test) {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return nil, nil, nil
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ppfns, should.BeEmpty)
		})
		t.Run("Mutate returns no transitions, but some semantic garbage is still cleaned up", func(t *ftt.Test) {
			assert.Loosely(t, Emit(ctx, []byte("msg"), recipient), should.BeNil)
			assert.Loosely(t, Emit(ctx, []byte("msg"), recipient), should.BeNil)
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return nil, es[:1], nil
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ppfns, should.BeEmpty)
			l, err := List(ctx, recipient)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, l, should.HaveLength(1))
			assert.Loosely(t, l[0].Value, should.Resemble([]byte("msg")))
		})
		t.Run("Garbage is cleaned up even if Mutate also returns error", func(t *ftt.Test) {
			assert.Loosely(t, Emit(ctx, []byte("msg"), recipient), should.BeNil)
			assert.Loosely(t, Emit(ctx, []byte("msg"), recipient), should.BeNil)
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return nil, es[:1], errors.New("boom")
			}
			_, err := ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, err, should.ErrLike("boom"))
			l, err := List(ctx, recipient)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, l, should.HaveLength(1))
			assert.Loosely(t, l[0].Value, should.Resemble([]byte("msg")))
		})
		t.Run("Mutate returns empty slice of transitions", func(t *ftt.Test) {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return []Transition{}, nil, nil
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ppfns, should.BeEmpty)
		})
		t.Run("Mutate returns noop transitions only", func(t *ftt.Test) {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return []Transition{
					{TransitionTo: s},
				}, nil, nil
			}
			ppfns, err := ProcessBatch(ctx, recipient, p, limit)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ppfns, should.BeEmpty)
		})

		t.Run("Test's own sanity check that fetchEVersion is called and panics", func(t *ftt.Test) {
			p.prepareMutation = func(_ context.Context, es Events, s State) ([]Transition, Events, error) {
				return []Transition{
					{TransitionTo: new(int)},
				}, nil, nil
			}
			assert.Loosely(t, func() { ProcessBatch(ctx, recipient, p, limit) }, should.PanicLike(panicErr))
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

func TestPrepareMutation(t *testing.T) {
	t.Parallel()

	ftt.Run("Chain of SideEffectFn works", t, func(t *ftt.Test) {
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
		t.Run("all nils chain to nil", func(t *ftt.Test) {
			assert.Loosely(t, Chain(), should.BeNil)
			assert.Loosely(t, Chain(nil), should.BeNil)
			assert.Loosely(t, Chain(nil, nil), should.BeNil)
		})
		t.Run("order is respected", func(t *ftt.Test) {
			assert.Loosely(t, Chain(nil, f2, nil, f1, f2, f1, nil)(ctx), should.BeNil)
			assert.Loosely(t, ops, should.Resemble([]string{"f2", "f1", "f2", "f1"}))
		})
		t.Run("error aborts", func(t *ftt.Test) {
			assert.Loosely(t, Chain(f1, nil, ferr, f2)(ctx), should.ErrLike(breakChain))
			assert.Loosely(t, ops, should.Resemble([]string{"f1", "ferr"}))
		})
	})
}
