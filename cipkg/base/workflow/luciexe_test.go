// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workflow

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/luciexe/build"
)

func TestRootStep(t *testing.T) {
	ftt.Run("Test RootStep", t, func(t *ftt.Test) {
		ctx := context.Background()
		state, stepCtx, err := build.Start(ctx, nil)
		assert.Loosely(t, err, should.BeNil)
		defer func() { state.End(nil) }()

		t.Run("canceled substep context does not block RootStep", func(t *ftt.Test) {
			root := NewRootStep(stepCtx, "root", "root-id")
			subCtx, cancel := context.WithCancel(stepCtx)
			started := make(chan struct{})
			release := make(chan struct{})

			errCh := make(chan error, 1)
			go func() {
				errCh <- root.RunSubstep(subCtx, func(ctx context.Context, root *build.Step) error {
					close(started)
					<-release
					return nil
				})
			}()

			<-started
			cancel()
			assert.Loosely(t, <-errCh, should.NotBeNil)

			close(release)
			root.End()
			assert.Loosely(t, root.IsEnded(), should.BeTrue)
		})
	})
}
