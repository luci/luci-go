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

	"go.chromium.org/luci/common/exec/internal/execmockserver"
)

type execMocker[In any, Out any] struct {
	runnerID uint64
	f        filter
}

func (e *execMocker[In, Out]) Mock(ctx context.Context, indatas ...In) *Uses[Out] {
	return addMockEntry[Out](ctx, e.f, &execmockserver.InvocationInput{
		RunnerID:    e.runnerID,
		RunnerInput: getOne(indatas),
	})
}

func (e *execMocker[In, Out]) WithArgs(argPattern ...string) Mocker[In, Out] {
	ret := *e
	ret.f = e.f.withArgs(argPattern)
	return &ret
}

func (e *execMocker[In, Out]) WithEnv(varName, valuePattern string) Mocker[In, Out] {
	ret := *e
	ret.f = e.f.withEnv(varName, valuePattern)
	return &ret
}

func (e *execMocker[In, Out]) WithLimit(limit uint64) Mocker[In, Out] {
	ret := *e
	ret.f.limit = limit
	return &ret
}
