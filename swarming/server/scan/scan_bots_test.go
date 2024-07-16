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
	"sort"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

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

type testBotVisitor struct {
	t       *testing.T
	shards  [][]string
	visited stringset.Set
}

func (v *testBotVisitor) Prepare(ctx context.Context, shards int) {
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

	v.Prepare(ctx, max(len(shards), 1))

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

	Pool []string
	OS   []string
	Dims []string

	Maintenance bool
	Dead        bool
	Quarantined bool
	Busy        bool

	Handshaking   bool
	RBEInstance   string
	RBEHybridMode bool
}

func (f *FakeBot) BotInfo(ctx context.Context) *model.BotInfo {
	b := &model.BotInfo{Key: model.BotInfoKey(ctx, f.ID)}
	if !f.NoLastSeen {
		b.LastSeen = datastore.NewUnindexedOptional(time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC))
	}

	if !f.NoState {
		var state struct {
			Handshaking   bool   `json:"handshaking"`
			RBEInstance   string `json:"rbe_instance"`
			RBEHybridMode bool   `json:"rbe_hybrid_mode"`
		}
		state.Handshaking = f.Handshaking
		state.RBEInstance = f.RBEInstance
		state.RBEHybridMode = f.RBEHybridMode
		var err error
		b.State, err = json.Marshal(&state)
		if err != nil {
			panic(err)
		}
	}

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
