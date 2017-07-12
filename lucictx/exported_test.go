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

	"github.com/luci/luci-go/common/system/environ"
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

		le := &liveExport{path: tfn}

		Convey("Can only be closed once", func() {
			le.Close()
			So(func() { le.Close() }, ShouldPanic)
		})

		Convey("Removes the file when it is closed", func() {
			_, err := os.Stat(tfn)
			So(err, ShouldBeNil)

			le.Close()
			_, err = os.Stat(tfn)
			So(os.IsNotExist(err), ShouldBeTrue)
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
