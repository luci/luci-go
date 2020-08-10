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

package testutil

import (
	"context"
	"sort"
	"sync"

	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/reminder"
)

var fakeDBKey = "FakeDB"

func init() {
	db.Register(db.Impl{
		Kind: "FakeDB",
		ProbeForTxn: func(ctx context.Context) db.DB {
			if db, _ := ctx.Value(&fakeDBKey).(*FakeDB); db != nil {
				return db
			}
			return nil
		},
		NonTxn: func(ctx context.Context) db.DB {
			if db, _ := ctx.Value(&fakeDBKey).(*FakeDB); db != nil {
				return db
			}
			return &FakeDB{} // assume the DB empty otherwise
		},
	})
}

// FakeDB implements Database in RAM.
type FakeDB struct {
	mu        sync.RWMutex
	reminders map[string]*reminder.Reminder
	defers    []func(context.Context)
}

func (f *FakeDB) Kind() string { return "FakeDB" }

func (f *FakeDB) Defer(_ context.Context, cb func(context.Context)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.defers = append(f.defers, cb)
}

func (f *FakeDB) SaveReminder(_ context.Context, r *reminder.Reminder) error {
	f.mu.Lock()
	if f.reminders == nil {
		f.reminders = map[string]*reminder.Reminder{}
	}
	f.reminders[r.ID] = r
	f.mu.Unlock()
	return nil
}

func (f *FakeDB) DeleteReminder(_ context.Context, r *reminder.Reminder) error {
	f.mu.Lock()
	if f.reminders == nil {
		return nil
	}
	for id, _ := range f.reminders {
		if id == r.ID {
			delete(f.reminders, id)
			break
		}
	}
	f.mu.Unlock()
	return nil
}

func (f *FakeDB) FetchRemindersMeta(ctx context.Context, low, high string, limit int) ([]*reminder.Reminder, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.reminders == nil {
		return nil, nil
	}
	var ids []string
	for id, _ := range f.reminders {
		if low <= id && id < high {
			// Optimal algo uses a heap of size exactly limit, but Go makes it very
			// verbose, so simple stupid sort.
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	ret := make([]*reminder.Reminder, len(ids))
	for i, id := range ids {
		ret[i] = &reminder.Reminder{
			ID:         id,
			FreshUntil: f.reminders[id].FreshUntil,
		}
	}
	return ret, nil
}

func (f *FakeDB) FetchReminderPayloads(_ context.Context, in []*reminder.Reminder) (out []*reminder.Reminder, err error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.reminders == nil {
		return
	}
	out = make([]*reminder.Reminder, 0, len(in))
	for _, r := range in {
		if saved, exists := f.reminders[r.ID]; exists {
			r.Payload = saved.Payload
			r.Extra = saved.Extra
			out = append(out, r)
		}
	}
	return
}

// Not part of Database interface, but useful in tests.

// Inject inserts `f` into the context to make it transactional.
func (f *FakeDB) Inject(ctx context.Context) context.Context {
	return context.WithValue(ctx, &fakeDBKey, f)
}

// AllReminders returns all currently saved reminders.
func (f *FakeDB) AllReminders() []*reminder.Reminder {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.reminders == nil {
		return nil
	}
	out := make([]*reminder.Reminder, 0, len(f.reminders))
	for _, r := range f.reminders {
		out = append(out, &reminder.Reminder{
			ID:         r.ID,
			FreshUntil: r.FreshUntil,
			Payload:    r.Payload,
			Extra:      r.Extra,
		})
	}
	return out
}

// ExecDefers executes all registered defers in reverse order.
func (f *FakeDB) ExecDefers(ctx context.Context) {
	f.mu.Lock()
	defers := f.defers
	f.defers = nil
	f.mu.Unlock()
	for i := len(defers) - 1; i >= 0; i-- {
		defers[i](ctx)
	}
}
