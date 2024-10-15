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
	"testing"

	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/exec/internal/execmockctx"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	ftt.Run(`filter`, t, func(t *ftt.Test) {
		t.Run(`Args`, func(t *ftt.Test) {
			t.Run(`valid`, func(t *ftt.Test) {
				e := filter{}.withArgs([]string{"hello", "/(there|you)/"})
				assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Args: []string{"hello", "there"}}), should.BeTrue)
				assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Args: []string{"hello", "you"}}), should.BeTrue)
				assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Args: []string{"hello", "sir"}}), should.BeFalse)
			})

			t.Run(`invalid`, func(t *ftt.Test) {
				assert.Loosely(t, func() {
					filter{}.withArgs([]string{"/*/"})
				}, should.PanicLike("invalid regexp"))
			})
		})

		t.Run(`Env`, func(t *ftt.Test) {
			t.Run(`valid`, func(t *ftt.Test) {
				t.Run(`single`, func(t *ftt.Test) {
					e := filter{}.withEnv("SOMETHING", "/cool.beans/")
					assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans"})}), should.BeTrue)
					assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool?beans"})}), should.BeTrue)
					assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool pintos"})}), should.BeFalse)

					t.Run(`double`, func(t *ftt.Test) {
						e = e.withEnv("OTHER", "nerds")
						assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans"})}), should.BeFalse)
						assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans", "OTHER=nerds"})}), should.BeTrue)
					})

					t.Run(`negative`, func(t *ftt.Test) {
						e = e.withEnv("OTHER", "!")
						assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans"})}), should.BeTrue)
						assert.Loosely(t, e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans", "OTHER=nerds"})}), should.BeFalse)
					})
				})
			})

			t.Run(`invalid`, func(t *ftt.Test) {
				assert.Loosely(t, func() {
					filter{}.withEnv("SOMETHING", "/*/")
				}, should.PanicLike("invalid regexp"))
			})
		})

		t.Run(`Less`, func(t *ftt.Test) {
			// TODO: Test me
		})
	})
}

func TestContext(t *testing.T) {
	t.Parallel()

	ftt.Run(`context`, t, func(t *ftt.Test) {
		ctx := Init(context.Background())

		t.Run(`uninitialized`, func(t *ftt.Test) {
			err := exec.Command(context.Background(), "echo", "hello").Run()
			assert.Loosely(t, err, should.ErrLike(execmockctx.ErrNoMatchingMock))
			assert.Loosely(t, err, should.ErrLike("execmock.Init not called on context"))
		})

		t.Run(`zero`, func(t *ftt.Test) {
			err := exec.Command(ctx, "echo", "hello").Run()
			assert.Loosely(t, err, should.ErrLike(execmockctx.ErrNoMatchingMock))

			misses := ResetState(ctx)
			assert.Loosely(t, misses, should.HaveLength(1))
			assert.Loosely(t, misses[0].Args, should.Resemble([]string{"echo", "hello"}))
		})

		t.Run(`single`, func(t *ftt.Test) {
			uses := Simple.Mock(ctx)

			err := exec.Command(ctx, "echo", "hello").Run()
			assert.Loosely(t, err, should.BeNil)

			usages := uses.Snapshot()
			assert.Loosely(t, usages, should.HaveLength(1))
			assert.Loosely(t, usages[0].Args, should.Resemble([]string{"echo", "hello"}))
			assert.Loosely(t, usages[0].GetPID(), should.NotEqual(0))
		})

		t.Run(`multi`, func(t *ftt.Test) {
			generalUsage := Simple.Mock(ctx)
			specificUsage := Simple.WithArgs("echo").Mock(ctx, SimpleInput{Stdout: "mocky mock"})

			out, err := exec.Command(ctx, "echo", "hello").CombinedOutput()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.Resemble([]byte("mocky mock")))

			assert.Loosely(t, generalUsage.Snapshot(), should.BeEmpty)

			usages := specificUsage.Snapshot()
			assert.Loosely(t, usages, should.HaveLength(1))
			assert.Loosely(t, usages[0].Args, should.Resemble([]string{"echo", "hello"}))
			assert.Loosely(t, usages[0].GetPID(), should.NotEqual(0))
		})

		t.Run(`multi (limit)`, func(t *ftt.Test) {
			generalUsage := Simple.Mock(ctx)
			specificUsage := Simple.WithArgs("echo").WithLimit(1).Mock(ctx, SimpleInput{Stdout: "mocky mock"})

			// fully consumes specificUsage
			out, err := exec.Command(ctx, "echo", "hello").CombinedOutput()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.Resemble([]byte("mocky mock")))

			// falls into generalUsage mock
			out, err = exec.Command(ctx, "echo", "hello").CombinedOutput()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.Resemble([]byte("")))

			assert.Loosely(t, generalUsage.Snapshot(), should.HaveLength(1))
			assert.Loosely(t, specificUsage.Snapshot(), should.HaveLength(1))
		})
	})
}

func TestGobName(t *testing.T) {
	t.Parallel()

	ftt.Run(`gobName`, t, func(t *ftt.Test) {
		assert.Loosely(t, gobName(SimpleInput{}), should.Match("go.chromium.org/luci/common/exec/execmock.SimpleInput"))
		assert.Loosely(t, gobName(&SimpleInput{}), should.Match("*go.chromium.org/luci/common/exec/execmock.SimpleInput"))

		assert.Loosely(t, gobName(100), should.Match("int"))
	})
}
