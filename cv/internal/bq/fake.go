// Copyright 2021 The LUCI Authors.
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

package bq

import (
	"context"
	"sync"

	"google.golang.org/protobuf/proto"
)

// Fake is a fake BQ client for tests.
type Fake struct {
	// mu protects access/mutation to this Fake.
	mu sync.RWMutex
	// sent is a map of "dataset.table" keys to slices of sent rows.
	sent map[string][]proto.Message
}

// SendRow provides a mock SendRow implementation for tests.
func (f *Fake) SendRow(ctx context.Context, dataset, table, operationID string, row proto.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := dataset + "." + table
	f.sent[key] = append(f.sent[key], row)
	return nil
}

// Rows returns the stored rows for a given dataset and table.
func (f *Fake) Rows(dataset, table string) []proto.Message {
	f.mu.RLock()
	defer f.mu.RUnlock()
	rows := f.sent[dataset+"."+table]
	ret := make([]proto.Message, len(rows))
	copy(ret, rows)
	return ret
}

// Ensure that Fake implement the Client interface.
var _ Client = (*Fake)(nil)

// Install installs this fake BQ client into context.
func (f *Fake) Install(ctx context.Context) context.Context {
	if f == nil {
		panic("nil bq fake")
	}
	return Install(ctx, &Fake{})
}
