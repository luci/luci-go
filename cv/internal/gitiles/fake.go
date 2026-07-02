// Copyright 2026 The LUCI Authors.
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

package gitiles

import (
	"context"
	"sync"

	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
)

// FakeFactory implements Factory for testing.
type FakeFactory struct {
	mu    sync.Mutex
	fakes map[string]*gitilespb.Fake
}

// NewFakeFactory returns a FakeFactory.
func NewFakeFactory() *FakeFactory {
	return &FakeFactory{
		fakes: make(map[string]*gitilespb.Fake),
	}
}

// MakeClient implements Factory.
func (f *FakeFactory) MakeClient(ctx context.Context, host, luciProject string) (Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fake, ok := f.fakes[host]
	if !ok {
		fake = &gitilespb.Fake{}
		f.fakes[host] = fake
	}
	return fake, nil
}

// SetRepository sets the repository state for a given host and project.
func (f *FakeFactory) SetRepository(host, project string, refs map[string]string, commits []*git.Commit) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fake, ok := f.fakes[host]
	if !ok {
		fake = &gitilespb.Fake{}
		f.fakes[host] = fake
	}
	fake.SetRepository(project, refs, commits)
}
