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

type startErrorMocker struct {
	f filter
}

func (e *startErrorMocker) Mock(ctx context.Context, errs ...error) *Uses[None] {
	return addMockEntry[None](ctx, e.f, &execmockserver.InvocationInput{
		StartError: getOne(errs),
	})
}

func (e *startErrorMocker) WithArgs(argPattern ...string) Mocker[error, None] {
	ret := *e
	ret.f = e.f.withArgs(argPattern)
	return &ret
}

func (e *startErrorMocker) WithEnv(varName, valuePattern string) Mocker[error, None] {
	ret := *e
	ret.f = e.f.withEnv(varName, valuePattern)
	return &ret
}

func (e *startErrorMocker) WithLimit(limit uint64) Mocker[error, None] {
	ret := *e
	ret.f.limit = limit
	return &ret
}

// StartError allows you to mock any execution with an error which will be
// returned from Start() (i.e. no process will actually run).
//
// This can be used to return exec.ErrNotFound from a given invocation, or any
// other error (e.g. bad executable file, or whatever you like).
var StartError Mocker[error, None] = &startErrorMocker{}
