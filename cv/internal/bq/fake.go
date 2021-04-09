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
	// sent is the list of rows that have been sent.
	sent []proto.Message
	// mu protects access/mutation to this fake Tree.
	mu sync.RWMutex
}

// SendRow provides a mock SendRow implementation for tests.
func (f *Fake) SendRow(ctx context.Context, dataset, table, operationID string, row proto.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, row)
	return nil
}

func (f *Fake) Rows() []proto.Message {
	return f.sent
}

// Ensure that Fake implement the Client interface.
var _ Client = (*Fake)(nil)

// Install installs this fake BQ client into context.
func (f *Fake) Install(ctx context.Context) context.Context {
	if f == nil {
		panic("nil tree fake")
	}
	return Install(ctx, &Fake{})
}
