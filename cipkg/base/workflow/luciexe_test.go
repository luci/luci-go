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
	"errors"
	"testing"

	"golang.org/x/time/rate"

	bbpb "go.chromium.org/luci/buildbucket/proto"
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

		t.Run("End without RunSubstep does not start step", func(t *ftt.Test) {
			root := NewRootStep(stepCtx, "unstarted-root", "unstarted-id")
			root.End()
			assert.Loosely(t, root.IsEnded(), should.BeTrue)
			assert.Loosely(t, root.RunSubstep(stepCtx, func(ctx context.Context, root *build.Step) error {
				return nil
			}), should.ErrLike("root step ended"))
		})

		t.Run("nested RunSubstep on same RootStep nests under current step", func(t *ftt.Test) {
			var latest *bbpb.Build
			st, bCtx, err := build.Start(ctx, nil, build.OptSend(rate.Inf, func(_ int64, b *bbpb.Build) {
				latest = b
			}))
			assert.NoErr(t, err)

			root := NewRootStep(bCtx, "root", "root-id")
			var outerRoot, innerRoot *build.Step
			err = root.RunSubstep(bCtx, func(ctx context.Context, r *build.Step) (err error) {
				outerRoot = r
				s, ctx := build.StartStep(ctx, "outer")
				defer func() { s.End(err) }()
				return root.RunSubstep(ctx, func(ctx context.Context, r *build.Step) (err error) {
					innerRoot = r
					is, _ := build.StartStep(ctx, "inner")
					defer func() { is.End(err) }()
					return nil
				})
			})
			assert.NoErr(t, err)
			assert.Loosely(t, innerRoot, should.Equal(outerRoot))

			root.End()
			st.End(nil)

			var names []string
			for _, s := range latest.GetSteps() {
				names = append(names, s.GetName())
			}
			assert.Loosely(t, names, should.Match([]string{
				"root",
				"root|outer",
				"root|outer|inner",
			}))
		})

		t.Run("nested RunSubstep on different RootStep dispatches to target root", func(t *ftt.Test) {
			var latest *bbpb.Build
			st, bCtx, err := build.Start(ctx, nil, build.OptSend(rate.Inf, func(_ int64, b *bbpb.Build) {
				latest = b
			}))
			assert.NoErr(t, err)

			root1 := NewRootStep(bCtx, "root1", "root1-id")
			root2 := NewRootStep(bCtx, "root2", "root2-id")
			err = root1.RunSubstep(bCtx, func(ctx context.Context, _ *build.Step) (err error) {
				s, ctx := build.StartStep(ctx, "outer")
				defer func() { s.End(err) }()
				return root2.RunSubstep(ctx, func(ctx context.Context, _ *build.Step) (err error) {
					is, _ := build.StartStep(ctx, "inner")
					defer func() { is.End(err) }()
					return nil
				})
			})
			assert.NoErr(t, err)

			root1.End()
			root2.End()
			st.End(nil)

			var names []string
			for _, s := range latest.GetSteps() {
				names = append(names, s.GetName())
			}
			assert.Loosely(t, names, should.Contain("root1|outer"))
			assert.Loosely(t, names, should.Contain("root2|inner"))
		})

		t.Run("nested RunSubstep error propagates", func(t *ftt.Test) {
			root := NewRootStep(stepCtx, "root-err", "root-err-id")
			wantErr := errors.New("boom")
			err := root.RunSubstep(stepCtx, func(ctx context.Context, _ *build.Step) error {
				return root.RunSubstep(ctx, func(ctx context.Context, _ *build.Step) error {
					return wantErr
				})
			})
			assert.Loosely(t, errors.Is(err, wantErr), should.BeTrue)
			root.End()
			assert.Loosely(t, errors.Is(root.Err(), wantErr), should.BeTrue)
		})
	})
}
