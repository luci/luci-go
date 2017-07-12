// Copyright 2015 The LUCI Authors.
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

package tumble

import (
	"fmt"
	"testing"

	ds "github.com/luci/gae/service/datastore"

	"golang.org/x/net/context"
)

func init() {
	Register(&registerBenchmarkMutation{})
}

type registerBenchmarkMutation struct {
	Parent *ds.Key
	ID     string
	Rounds int
}

func (m *registerBenchmarkMutation) RollForward(c context.Context) ([]Mutation, error) {
	type testEntity struct {
		Parent *ds.Key `gae:"$parent"`
		Kind   string  `gae:"$kind,test"`
		ID     string  `gae:"$id"`

		Name string
	}

	// Perform a Get, Query, then Put.
	e := testEntity{Parent: m.Root(c), ID: m.ID}
	if err := ds.Get(c, &e); err != nil && err != ds.ErrNoSuchEntity {
		return nil, err
	}

	var all []*testEntity
	if err := ds.GetAll(c, ds.NewQuery("test").Ancestor(m.Parent), &all); err != nil {
		return nil, err
	}

	ents := make([]*testEntity, m.Rounds)
	for i := range ents {
		ents[i] = &testEntity{Parent: m.Parent, ID: fmt.Sprintf("%s.%d", m.ID, i)}
	}
	if err := ds.Put(c, ents); err != nil {
		return nil, err
	}

	// Put the entity, add some new Mutations at the end.
	return []Mutation{
		&registerBenchmarkMutation{m.Parent, "bar", m.Rounds},
		&registerBenchmarkMutation{m.Parent, "baz", m.Rounds},
	}, nil
}

func (m *registerBenchmarkMutation) Root(c context.Context) *ds.Key { return m.Parent }

func runRegisterBenchmark(b *testing.B, reg func(context.Context, Mutation) error) {
	var t Testing
	c := t.Context()
	mut := registerBenchmarkMutation{ds.MakeKey(c, "parent", 1), "foo", 500}

	b.ResetTimer()
	if err := reg(c, &mut); err != nil {
		panic(err)
	}
}

func BenchmarkRegisterWithRunMutation(b *testing.B) {
	runRegisterBenchmark(b, RunMutation)
}

func BenchmarkRegisterWithRunUnbuffered(b *testing.B) {
	runRegisterBenchmark(b, func(c context.Context, m Mutation) error {
		return RunUnbuffered(c, m.Root(c), m.RollForward)
	})
}

//BenchmarkRegisterWithRunMutation-32             1000000000       0.52 ns/op
//BenchmarkRegisterWithRunUnbuffered-32           1000000000       0.22 ns/op
