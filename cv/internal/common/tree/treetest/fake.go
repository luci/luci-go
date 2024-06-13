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

// Package treetest implements fake Tree for testing in CV.
package treetest

import (
	"context"
	"sync"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/cv/internal/common/tree"
)

// Fake simulates Tree status app in test.
type Fake struct {
	// TreeStatus represents the current status of this fake Tree.
	TreeStatus tree.Status
	// Error forces FetchLatest to fail when set.
	Error error
	// mu protects access/mutation to this fake Tree.
	mu sync.RWMutex
}

// NewFake returns a fake Tree.
func NewFake(ctx context.Context, state tree.State) *Fake {
	return &Fake{
		TreeStatus: tree.Status{
			State: state,
			Since: clock.Now(ctx).UTC(),
		},
	}
}

// Client returns a client of this Fake Tree.
func (f *Fake) Client() tree.Client {
	return &client{f}
}

// InjectErr causes Fake tree status app to return error.
//
// Passing nil error will bring tree status app to normal.
func (f *Fake) InjectErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Error = err
}

// ModifyState changes the state of this fake Tree.
func (f *Fake) ModifyState(ctx context.Context, newState tree.State) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.TreeStatus.State != newState {
		f.TreeStatus.State = newState
		f.TreeStatus.Since = clock.Now(ctx).UTC()
	}
}

type client struct {
	fake *Fake
}

var _ tree.Client = (*client)(nil)

func (c *client) FetchLatest(ctx context.Context, treeName string) (tree.Status, error) {
	c.fake.mu.RLock()
	defer c.fake.mu.RUnlock()
	if c.fake.Error != nil {
		return tree.Status{}, c.fake.Error
	}
	return c.fake.TreeStatus, nil
}
