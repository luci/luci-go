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

package internal

import (
	"context"
	"sort"
	"sync"
)

// FakeDB implements Database in RAM.
// Used for testing of ttq guts.
// TODO(tandrii): move into testing/ package after
// moving Reminder to a new package to avoid circular dependency.
type FakeDB struct {
	mu        sync.RWMutex
	reminders map[string]*Reminder
}

func (_ *FakeDB) Kind() string { return "FakeDB" }

func (f *FakeDB) SaveReminder(_ context.Context, r *Reminder) error {
	f.mu.Lock()
	if f.reminders == nil {
		f.reminders = map[string]*Reminder{}
	}
	f.reminders[r.Id] = r
	f.mu.Unlock()
	return nil
}

func (f *FakeDB) DeleteReminder(_ context.Context, r *Reminder) error {
	f.mu.Lock()
	if f.reminders == nil {
		return nil
	}
	for id, _ := range f.reminders {
		if id == r.Id {
			delete(f.reminders, id)
			break
		}
	}
	f.mu.Unlock()
	return nil
}

func (f *FakeDB) FetchRemindersMeta(ctx context.Context, low, high string, limit int) ([]*Reminder, error) {
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
	ret := make([]*Reminder, len(ids))
	for i, id := range ids {
		ret[i] = &Reminder{
			Id:         id,
			FreshUntil: f.reminders[id].FreshUntil,
		}
	}
	return ret, nil
}

func (f *FakeDB) FetchReminderPayloads(_ context.Context, in []*Reminder) (out []*Reminder, err error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.reminders == nil {
		return
	}
	out = make([]*Reminder, 0, len(in))
	for _, r := range in {
		if saved, exists := f.reminders[r.Id]; exists {
			r.Payload = saved.Payload
			out = append(out, r)
		}
	}
	return
}

// Not part of Database interface, but useful in tests.

// AllReminders returns all currently saved reminders.
func (f *FakeDB) AllReminders() []*Reminder {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.reminders == nil {
		return nil
	}
	out := make([]*Reminder, 0, len(f.reminders))
	for _, r := range f.reminders {
		out = append(out, &Reminder{
			Id:         r.Id,
			FreshUntil: r.FreshUntil,
			Payload:    r.Payload,
		})
	}
	return out
}
