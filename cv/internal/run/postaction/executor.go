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

package postaction

import (
	"context"

	"go.chromium.org/luci/cv/internal/gerrit"
)

// Executor executes a PostAction for a Run termination event.
type Executor struct {
	gFactory gerrit.Factory
}

// NewExecutor creates a new Executor.
func NewExecutor(gf gerrit.Factory) *Executor {
	return &Executor{gFactory: gf}
}

// Do executes the payload.
func (exe *Executor) Do(ctx context.Context, payload *ExecutePostActionPayload) error {
	// TODO(ddoman): implement me
	return nil
}
