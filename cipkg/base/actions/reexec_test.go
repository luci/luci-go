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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/testutils"
)

func TestReexecDerivation(t *testing.T) {
	ftt.Run("Test ReexecDerivation", t, func(t *ftt.Test) {
		t.Setenv("UNRELATED_HOST_ENV", "should-not-leak")
		spec := &core.ActionURLFetch{Url: "https://example.com/file"}

		t.Run("explicit env does not leak host env, mutate caller, or affect FixedOutput", func(t *ftt.Test) {
			in := environ.New([]string{"FOO=bar"})
			drv1, err := ReexecDerivation(spec, nil, in)
			assert.NoErr(t, err)
			drv2, err := ReexecDerivation(spec, nil, environ.New([]string{"FOO=baz", "EXTRA=1"}))
			assert.NoErr(t, err)
			drvZero, err := ReexecDerivation(spec, nil, environ.Env{})
			assert.NoErr(t, err)

			assert.That(t, drv1.FixedOutput, should.Equal(drv2.FixedOutput))
			assert.That(t, drv1.FixedOutput, should.Equal(drvZero.FixedOutput))
			assert.That(t, in.Get(envCipkgExec), should.Equal(""))

			env1 := environ.New(drv1.Env)
			assert.That(t, env1.Get("FOO"), should.Equal("bar"))
			assert.That(t, env1.Get(envCipkgExec), should.Equal("1"))
			assert.That(t, env1.Get("UNRELATED_HOST_ENV"), should.Equal(""))
		})

		t.Run("deps affect FixedOutput and populate dep env", func(t *ftt.Test) {
			pm := testutils.NewMockPackageManage("/store")
			dep1 := Package{
				Action:       &core.Action{Name: "dep"},
				DerivationID: "dep-111",
				Handler:      pm.Get("dep-111"),
			}
			dep2 := Package{
				Action:       &core.Action{Name: "dep"},
				DerivationID: "dep-222",
				Handler:      pm.Get("dep-222"),
			}

			drv1, err := ReexecDerivation(spec, []Package{dep1}, environ.Env{})
			assert.NoErr(t, err)
			drv2, err := ReexecDerivation(spec, []Package{dep2}, environ.Env{})
			assert.NoErr(t, err)
			drvNoDep, err := ReexecDerivation(spec, nil, environ.Env{})
			assert.NoErr(t, err)

			assert.That(t, drv1.FixedOutput, should.NotEqual(drv2.FixedOutput))
			assert.That(t, drv1.FixedOutput, should.NotEqual(drvNoDep.FixedOutput))
			assert.That(t, environ.New(drv1.Env).Get("dep"), should.Equal(dep1.Handler.OutputDirectory()))
		})
	})
}

func TestSetExecutor(t *testing.T) {
	ftt.Run("Test set executor", t, func(t *ftt.Test) {
		reg := NewReexecRegistry()

		t.Run("ok", func(t *ftt.Test) {
			err := SetExecutor(reg, func(ctx context.Context, msg *anypb.Any, out string) error { return nil })
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("duplicated", func(t *ftt.Test) {
			err := SetExecutor(reg, func(ctx context.Context, msg *anypb.Any, out string) error { return nil })
			assert.Loosely(t, err, should.BeNil)
			err = SetExecutor(reg, func(ctx context.Context, msg *anypb.Any, out string) error { return nil })
			assert.Loosely(t, errors.Is(err, ErrExecutorExisted), should.BeTrue)
		})

		t.Run("sealed", func(t *ftt.Test) {
			reg.interceptWithArgs(context.Background(), environ.New(nil), []string{}, func(i int) {})
			err := SetExecutor(reg, func(ctx context.Context, msg *anypb.Any, out string) error { return nil })
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

func execDrv(tb testing.TB, ctx context.Context, drv *core.Derivation, out string) error {
	tb.Helper()
	cmd := exec.CommandContext(ctx, drv.Args[0], drv.Args[1:]...)
	cmd.Env = append(slices.Clone(drv.Env), "out="+out)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, outBytes)
	}
	return nil
}

func clearProxyEnv(tb testing.TB) {
	tb.Helper()
	for _, k := range []string{
		"http_proxy", "HTTP_PROXY",
		"https_proxy", "HTTPS_PROXY",
		"all_proxy", "ALL_PROXY",
		"no_proxy", "NO_PROXY",
	} {
		tb.Setenv(k, "")
	}
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

		t.Chdir(bin)

		fname := filepath.Base(f.Name())

		err = os.Unsetenv("NoDefaultCurrentDirectoryInExePath")
		assert.Loosely(t, err, should.BeNil)

		_, err = exec.LookPath(fname)
		assert.Loosely(t, errors.Is(err, exec.ErrDot), should.BeTrue)

		t.Setenv("NoDefaultCurrentDirectoryInExePath", "1")

		_, err = exec.LookPath(fname)
		assert.Loosely(t, errors.Is(err, exec.ErrNotFound), should.BeTrue)
	})
}
