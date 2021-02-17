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

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/system/environ"
)

func TestLiveExported(t *testing.T) {
	// t.Parallel() because of os.Environ manipulation

	dir, err := ioutil.TempDir(os.TempDir(), "exported_test")
	if err != nil {
		t.Fatalf("could not create tempdir! %s", err)
	}
	defer os.RemoveAll(dir)

	Convey("LiveExports", t, func() {
		os.Unsetenv(EnvKey)

		tf, err := ioutil.TempFile(dir, "exported_test.liveExport")
		tfn := tf.Name()
		tf.Close()
		So(err, ShouldBeNil)
		defer os.Remove(tfn)

		closed := false
		le := &liveExport{
			path:   tfn,
			closer: func() { closed = true },
		}

		Convey("Can only be closed once", func() {
			le.Close()
			So(func() { le.Close() }, ShouldPanic)
		})

		Convey("Calls closer when it is closed", func() {
			So(closed, ShouldBeFalse)
			le.Close()
			So(closed, ShouldBeTrue)
		})

		Convey("Can add to command", func() {
			cmd := exec.Command("test", "arg")
			cmd.Env = os.Environ()
			le.SetInCmd(cmd)
			So(len(cmd.Env), ShouldEqual, len(os.Environ())+1)
			So(cmd.Env[len(cmd.Env)-1], ShouldStartWith, EnvKey)
			So(cmd.Env[len(cmd.Env)-1], ShouldEndWith, le.path)
		})

		Convey("Can modify in command", func() {
			cmd := exec.Command("test", "arg")
			cmd.Env = os.Environ()
			cmd.Env[0] = EnvKey + "=helloworld"
			le.SetInCmd(cmd)
			So(len(cmd.Env), ShouldEqual, len(os.Environ()))
			So(cmd.Env[0], ShouldStartWith, EnvKey)
			So(cmd.Env[0], ShouldEndWith, le.path)
		})

		Convey("Can add to environ", func() {
			env := environ.System()
			_, ok := env.Get(EnvKey)
			So(ok, ShouldBeFalse)
			le.SetInEnviron(env)
			val, ok := env.Get(EnvKey)
			So(ok, ShouldBeTrue)
			So(val, ShouldEqual, le.path)
		})
	})
}

func TestNullExported(t *testing.T) {
	t.Parallel()

	n := nullExport{}
	someEnv := []string{"SOME_STUFF=0"}

	Convey("SetInCmd, no LUCI_CONTEXT", t, func() {
		cmd := exec.Cmd{
			Env: append([]string(nil), someEnv...),
		}
		n.SetInCmd(&cmd)
		So(cmd.Env, ShouldResemble, someEnv)
	})

	Convey("SetInCmd, with LUCI_CONTEXT", t, func() {
		cmd := exec.Cmd{
			Env: append([]string{"LUCI_CONTEXT=abc"}, someEnv...),
		}
		n.SetInCmd(&cmd)
		So(cmd.Env, ShouldResemble, someEnv) // no LUCI_CONTEXT anymore
	})

	Convey("SetInEnviron, no LUCI_CONTEXT", t, func() {
		env := environ.New(someEnv)
		n.SetInEnviron(env)
		So(env.Sorted(), ShouldResemble, someEnv)
	})

	Convey("SetInEnviron, with LUCI_CONTEXT", t, func() {
		env := environ.New(append([]string{"LUCI_CONTEXT=abc"}, someEnv...))
		n.SetInEnviron(env)
		So(env.Sorted(), ShouldResemble, someEnv) // no LUCI_CONTEXT anymore
	})
}
