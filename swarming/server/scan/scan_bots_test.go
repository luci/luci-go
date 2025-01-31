// Copyright 2024 The LUCI Authors.
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

package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/model"
)

func TestBots(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	putBot := func(b FakeBot) {
		err := datastore.Put(ctx, b.BotInfo(ctx))
		if err != nil {
			t.Fatalf("Storing bot: %s", err)
		}
	}

	putBot(FakeBot{ID: "skip 0", NoState: true, NoLastSeen: true})
	putBot(FakeBot{ID: "skip 1", NoState: false, NoLastSeen: true})
	putBot(FakeBot{ID: "skip 2", NoState: true, NoLastSeen: false})

	// Many-many bots to make sure we engage logic related to multiple shards.
	const botCount = 10000
	for i := 0; i < botCount; i++ {
		putBot(FakeBot{ID: fmt.Sprintf("bot %d", i)})
	}

	v1 := &testBotVisitor{t: t}
	v2 := &testBotVisitor{t: t}

	err := Bots(ctx, []BotVisitor{v1, v2})
	if err != nil {
		t.Fatalf("%s", err)
	}

	if len(v1.visited) != botCount {
		t.Errorf("Expected %d bots, got %d", botCount, len(v1.visited))
	}
	if len(v2.visited) != botCount {
		t.Errorf("Expected %d bots, got %d", botCount, len(v2.visited))
	}
}

func TestBotScannerState(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	state, err := fetchState(ctx)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if state == nil || len(state) != 0 {
		t.Fatalf("should be non-nil empty dict")
	}

	var testTime = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)

	state.bumpLastScan("a", testTime)
	state.bumpLastScan("b", testTime)

	if err := storeState(ctx, state); err != nil {
		t.Fatalf("%s", err)
	}

	state, err = fetchState(ctx)
	if err != nil {
		t.Fatalf("%s", err)
	}

	if !testTime.Equal(state.lastScan("a")) {
		t.Errorf("%v", state)
	}
	if !testTime.Equal(state.lastScan("b")) {
		t.Errorf("%v", state)
	}

	updatedTime := testTime.Add(time.Hour)
	state.bumpLastScan("a", updatedTime)
	if err := storeState(ctx, state); err != nil {
		t.Fatalf("%s", err)
	}

	state, err = fetchState(ctx)
	if err != nil {
		t.Fatalf("%s", err)
	}

	if !updatedTime.Equal(state.lastScan("a")) {
		t.Errorf("%v", state)
	}
	if !testTime.Equal(state.lastScan("b")) {
		t.Errorf("%v", state)
	}
}

func TestVisitorsToRun(t *testing.T) {
	t.Parallel()

	all := []BotVisitor{
		&testBotVisitor{id: "always", frequency: 0},
		&testBotVisitor{id: "1h", frequency: time.Hour},
		&testBotVisitor{id: "2h", frequency: 2 * time.Hour},
	}

	var testTime = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
	tick := func(dt time.Duration) {
		testTime = testTime.Add(dt)
	}

	state := botScannerState{}

	run := func() []string {
		var ran []string
		for _, v := range visitorsToRun(context.Background(), state, all, testTime) {
			state.bumpLastScan(v.ID(), testTime)
			ran = append(ran, v.ID())
		}
		return ran
	}

	// First runs all of them.
	if got := run(); !slices.Equal(got, []string{"always", "1h", "2h"}) {
		t.Fatalf("run = %v", got)
	}

	// A bit later runs only the one that always runs.
	tick(10 * time.Minute)
	if got := run(); !slices.Equal(got, []string{"always"}) {
		t.Fatalf("run = %v", got)
	}

	// 1h since the initial start time, runs the 1h one.
	tick(50 * time.Minute)
	if got := run(); !slices.Equal(got, []string{"always", "1h"}) {
		t.Fatalf("run = %v", got)
	}

	// Finally runs the 2h one.
	tick(time.Hour)
	if got := run(); !slices.Equal(got, []string{"always", "1h", "2h"}) {
		t.Fatalf("run = %v", got)
	}

	// Now waiting again.
	tick(59 * time.Minute)
	if got := run(); !slices.Equal(got, []string{"always"}) {
		t.Fatalf("run = %v", got)
	}
}

type testBotVisitor struct {
	t         *testing.T
	id        string
	frequency time.Duration
	shards    [][]string
	visited   stringset.Set
}

func (v *testBotVisitor) ID() string {
	if v.id != "" {
		return v.id
	}
	return "testBotVisitor"
}

func (v *testBotVisitor) Frequency() time.Duration {
	return v.frequency
}

func (v *testBotVisitor) Prepare(ctx context.Context, shards int, lastTime time.Time) {
	if v.shards != nil {
		v.t.Error("Prepare called twice")
	}
	v.shards = make([][]string, shards)
}

func (v *testBotVisitor) Visit(ctx context.Context, shard int, bot *model.BotInfo) {
	v.shards[shard] = append(v.shards[shard], bot.BotID())
}

func (v *testBotVisitor) Finalize(ctx context.Context, scanErr error) error {
	v.visited = stringset.New(0)
	for _, s := range v.shards {
		for _, id := range s {
			if !v.visited.Add(id) {
				v.t.Errorf("Bot %s visited more than once", id)
			}
		}
	}
	return nil
}

/// Helpers used in other tests.

// RunBotVisitor invokes bot visitor the same way Bots(...) would.
func RunBotVisitor(ctx context.Context, v BotVisitor, b []FakeBot) error {
	const shardCount = 5

	// Split into chunks, each one will be processed in parallel.
	shardSize := max(len(b)/shardCount, 1)
	var shards [][]FakeBot
	for i := 0; i < len(b); i += shardSize {
		shards = append(shards, b[i:i+min(shardSize, len(b)-i)])
	}

	v.Prepare(ctx, max(len(shards), 1), time.Time{})

	var wg sync.WaitGroup
	wg.Add(len(shards))
	for shardIdx, shard := range shards {
		shardIdx := shardIdx
		shard := shard
		go func() {
			defer wg.Done()
			for _, bot := range shard {
				v.Visit(ctx, shardIdx, bot.BotInfo(ctx))
			}
		}()
	}

	wg.Wait()

	return v.Finalize(ctx, nil)
}

// FakeBot is used to construct model.BotInfo.
type FakeBot struct {
	ID         string
	NoLastSeen bool
	NoState    bool

	Pool    []string
	OS      []string
	Dims    []string
	State   []byte
	Version string

	Maintenance bool
	Dead        bool
	Quarantined bool
	Busy        bool

	LastSeen time.Time

	Handshaking   bool
	RBEInstance   string
	RBEHybridMode bool
}

func (f *FakeBot) BotInfo(ctx context.Context) *model.BotInfo {
	b := &model.BotInfo{
		Key: model.BotInfoKey(ctx, f.ID),
		BotCommon: model.BotCommon{
			Version: f.Version,
		},
	}
	if !f.NoLastSeen {
		lastSeen := f.LastSeen
		if lastSeen.IsZero() {
			lastSeen = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)
		}
		b.LastSeen = datastore.NewUnindexedOptional(lastSeen)
	}

	var s []byte
	switch {
	case !f.NoState && len(f.State) > 0:
		s = f.State
	case !f.NoState:
		var state struct {
			Handshaking   bool   `json:"handshaking"`
			RBEInstance   string `json:"rbe_instance"`
			RBEHybridMode bool   `json:"rbe_hybrid_mode"`
		}
		state.Handshaking = f.Handshaking
		state.RBEInstance = f.RBEInstance
		state.RBEHybridMode = f.RBEHybridMode
		var err error
		s, err = json.Marshal(&state)
		if err != nil {
			panic(err)
		}
	}
	b.State = botstate.Dict{JSON: s}

	var dims []string
	for _, pool := range f.Pool {
		dims = append(dims, "pool:"+pool)
	}
	for _, os := range f.OS {
		dims = append(dims, "os:"+os)
	}
	dims = append(dims, f.Dims...)
	sort.Strings(dims)
	b.Dimensions = dims

	b.Quarantined = f.Quarantined
	b.Composite = make([]model.BotStateEnum, 4)

	flip := func(idx int, toggle bool, yes, no model.BotStateEnum) {
		if toggle {
			b.Composite[idx] = yes
		} else {
			b.Composite[idx] = no
		}
	}
	flip(0, f.Maintenance, model.BotStateInMaintenance, model.BotStateNotInMaintenance)
	flip(1, f.Dead, model.BotStateDead, model.BotStateAlive)
	flip(2, f.Quarantined, model.BotStateQuarantined, model.BotStateHealthy)
	flip(3, f.Busy, model.BotStateBusy, model.BotStateIdle)

	return b
}
