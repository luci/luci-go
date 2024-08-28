// Copyright 2023 The LUCI Authors.
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

package actions

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
)

func TestSetExecutor(t *testing.T) {
	ftt.Run("Test set executor", t, func(t *ftt.Test) {
		reg := NewReexecRegistry()

		t.Run("ok", func(t *ftt.Test) {
			err := SetExecutor[*anypb.Any](reg, func(ctx context.Context, msg *anypb.Any, out string) error { return nil })
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("duplicated", func(t *ftt.Test) {
			err := SetExecutor[*anypb.Any](reg, func(ctx context.Context, msg *anypb.Any, out string) error { return nil })
			assert.Loosely(t, err, should.BeNil)
			err = SetExecutor[*anypb.Any](reg, func(ctx context.Context, msg *anypb.Any, out string) error { return nil })
			assert.Loosely(t, errors.Is(err, ErrExecutorExisted), should.BeTrue)
		})

		t.Run("sealed", func(t *ftt.Test) {
			reg.interceptWithArgs(context.Background(), environ.New(nil), []string{}, func(i int) {})
			err := SetExecutor[*anypb.Any](reg, func(ctx context.Context, msg *anypb.Any, out string) error { return nil })
			assert.Loosely(t, errors.Is(err, ErrReexecRegistrySealed), should.BeTrue)
		})
	})
}

func runWithDrv(tb testing.TB, ctx context.Context, drv *core.Derivation, out string) {
	env := environ.New(drv.Env)
	env.Set("out", out)
	code := -1
	NewReexecRegistry().interceptWithArgs(ctx, env, drv.Args, func(i int) {
		code = i
	})
	assert.Loosely(tb, code, should.BeZero)
}

func checkReexecArg(tb testing.TB, args []string, m proto.Message) {
	m, err := anypb.New(m)
	assert.Loosely(tb, err, should.BeNil)
	b, err := protojson.Marshal(m)
	assert.Loosely(tb, err, should.BeNil)
	assert.Loosely(tb, args, should.Contain(string(b)))
}

// This test is for windows's default binary searching behaviour, which lookup
// from current working directory. Because of its relative nature, this is
// forbidden by golang and causing error. We expect if Intercept() is called,
// NoDefaultCurrentDirectoryInExePath will be set to prevent that behaviour.
func TestBinLookup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.SkipNow()
	}

	ftt.Run("lookup from cwd", t, func(t *ftt.Test) {
		bin := t.TempDir()

		f, err := os.CreateTemp(bin, "something*.exe")
		assert.Loosely(t, err, should.BeNil)
		err = f.Close()
		assert.Loosely(t, err, should.BeNil)

		olddir, err := os.Getwd()
		assert.Loosely(t, err, should.BeNil)
		err = os.Chdir(bin)
		assert.Loosely(t, err, should.BeNil)
		t.Cleanup(func() {
			if err := os.Chdir(olddir); err != nil {
				t.Fatal(err)
			}
		})

		fname := filepath.Base(f.Name())

		err = os.Unsetenv("NoDefaultCurrentDirectoryInExePath")
		assert.Loosely(t, err, should.BeNil)

		_, err = exec.LookPath(fname)
		assert.Loosely(t, errors.Is(err, exec.ErrDot), should.BeTrue)

		err = os.Setenv("NoDefaultCurrentDirectoryInExePath", "1")
		assert.Loosely(t, err, should.BeNil)

		_, err = exec.LookPath(fname)
		assert.Loosely(t, errors.Is(err, exec.ErrNotFound), should.BeTrue)
	})
}
