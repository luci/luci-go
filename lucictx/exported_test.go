// Copyright 2016 The LUCI Authors.
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

package lucictx

import (
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLiveExported(t *testing.T) {
	// t.Parallel() because of os.Environ manipulation

	dir, err := ioutil.TempDir(os.TempDir(), "exported_test")
	if err != nil {
		t.Fatalf("could not create tempdir! %s", err)
	}
	defer os.RemoveAll(dir)

	ftt.Run("LiveExports", t, func(t *ftt.Test) {
		os.Unsetenv(EnvKey)

		tf, err := ioutil.TempFile(dir, "exported_test.liveExport")
		tfn := tf.Name()
		tf.Close()
		assert.Loosely(t, err, should.BeNil)
		defer os.Remove(tfn)

		closed := false
		le := &liveExport{
			path:   tfn,
			closer: func() { closed = true },
		}

		t.Run("Can only be closed once", func(t *ftt.Test) {
			le.Close()
			assert.Loosely(t, func() { le.Close() }, should.Panic)
		})

		t.Run("Calls closer when it is closed", func(t *ftt.Test) {
			assert.Loosely(t, closed, should.BeFalse)
			le.Close()
			assert.Loosely(t, closed, should.BeTrue)
		})

		t.Run("Can add to command", func(t *ftt.Test) {
			cmd := exec.Command("test", "arg")
			cmd.Env = os.Environ()
			le.SetInCmd(cmd)
			assert.Loosely(t, len(cmd.Env), should.Equal(len(os.Environ())+1))
			assert.Loosely(t, cmd.Env[len(cmd.Env)-1], should.HavePrefix(EnvKey))
			assert.Loosely(t, cmd.Env[len(cmd.Env)-1], should.HaveSuffix(le.path))
		})

		t.Run("Can modify in command", func(t *ftt.Test) {
			cmd := exec.Command("test", "arg")
			cmd.Env = os.Environ()
			cmd.Env[0] = EnvKey + "=helloworld"
			le.SetInCmd(cmd)
			assert.Loosely(t, len(cmd.Env), should.Equal(len(os.Environ())))
			assert.Loosely(t, cmd.Env[0], should.HavePrefix(EnvKey))
			assert.Loosely(t, cmd.Env[0], should.HaveSuffix(le.path))
		})

		t.Run("Can add to environ", func(t *ftt.Test) {
			env := environ.System()
			_, ok := env.Lookup(EnvKey)
			assert.Loosely(t, ok, should.BeFalse)
			le.SetInEnviron(env)
			val, ok := env.Lookup(EnvKey)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, val, should.Equal(le.path))
		})
	})
}

func TestNullExported(t *testing.T) {
	t.Parallel()

	n := nullExport{}
	someEnv := []string{"SOME_STUFF=0"}

	ftt.Run("SetInCmd, no LUCI_CONTEXT", t, func(t *ftt.Test) {
		cmd := exec.Cmd{
			Env: append([]string(nil), someEnv...),
		}
		n.SetInCmd(&cmd)
		assert.Loosely(t, cmd.Env, should.Resemble(someEnv))
	})

	ftt.Run("SetInCmd, with LUCI_CONTEXT", t, func(t *ftt.Test) {
		cmd := exec.Cmd{
			Env: append([]string{"LUCI_CONTEXT=abc"}, someEnv...),
		}
		n.SetInCmd(&cmd)
		assert.Loosely(t, cmd.Env, should.Resemble(someEnv)) // no LUCI_CONTEXT anymore
	})

	ftt.Run("SetInEnviron, no LUCI_CONTEXT", t, func(t *ftt.Test) {
		env := environ.New(someEnv)
		n.SetInEnviron(env)
		assert.Loosely(t, env.Sorted(), should.Resemble(someEnv))
	})

	ftt.Run("SetInEnviron, with LUCI_CONTEXT", t, func(t *ftt.Test) {
		env := environ.New(append([]string{"LUCI_CONTEXT=abc"}, someEnv...))
		n.SetInEnviron(env)
		assert.Loosely(t, env.Sorted(), should.Resemble(someEnv)) // no LUCI_CONTEXT anymore
	})
}
