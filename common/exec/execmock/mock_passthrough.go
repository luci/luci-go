// Copyright 2023 The LUCI Authors.
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

package execmock

import (
	"context"
)

type passthroughMocker struct {
	f filter
}

func (e *passthroughMocker) Mock(ctx context.Context, _ ...None) *Uses[None] {
	return addMockEntry[None](ctx, e.f, nil)
}

func (e *passthroughMocker) WithArgs(argPattern ...string) Mocker[None, None] {
	ret := *e
	ret.f = e.f.withArgs(argPattern)
	return &ret
}

func (e *passthroughMocker) WithEnv(varName, valuePattern string) Mocker[None, None] {
	ret := *e
	ret.f = e.f.withEnv(varName, valuePattern)
	return &ret
}

func (e *passthroughMocker) WithLimit(limit uint64) Mocker[None, None] {
	ret := *e
	ret.f.limit = limit
	return &ret
}

// Passthrough will make any matching commands just do normal, un-mocked,
// execution.
var Passthrough Mocker[None, None] = &passthroughMocker{}
