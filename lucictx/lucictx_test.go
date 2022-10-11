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
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/common/system/environ"
)

func rawctx(content string) func() {
	tf, err := ioutil.TempFile(os.TempDir(), "lucictx_test.")
	if err != nil {
		panic(err)
	}
	_, err = tf.WriteString(content)
	if err != nil {
		panic(err)
	}
	tf.Close()
	os.Setenv(EnvKey, tf.Name())

	return func() { os.Remove(tf.Name()) }
}

func TestLUCIContextInitialization(t *testing.T) {
	// t.Parallel() because of os.Environ manipulation

	Convey("extractFromEnv", t, func() {
		os.Unsetenv(EnvKey)
		buf := &bytes.Buffer{}

		Convey("works with missing envvar", func() {
			So(extractFromEnv(buf), ShouldResemble, &lctx{})
			So(buf.String(), ShouldEqual, "")
			_, ok := os.LookupEnv(EnvKey)
			So(ok, ShouldBeFalse)
		})

		Convey("works with bad envvar", func() {
			os.Setenv(EnvKey, "sup")
			So(extractFromEnv(buf), ShouldResemble, &lctx{})
			So(buf.String(), ShouldContainSubstring, "Could not open LUCI_CONTEXT file")
		})

		Convey("works with bad file", func() {
			defer rawctx(`"not a map"`)()

			So(extractFromEnv(buf), ShouldResemble, &lctx{})
			So(buf.String(), ShouldContainSubstring, "cannot unmarshal string into Go value")
		})

		Convey("ignores bad sections", func() {
			defer rawctx(`{"hi": {"hello_there": 10}, "not": "good"}`)()

			blob := json.RawMessage(`{"hello_there":10}`)
			So(extractFromEnv(buf).sections, ShouldResemble, map[string]*json.RawMessage{
				"hi": &blob,
			})
			So(buf.String(), ShouldContainSubstring, `section "not": Not a map`)
		})
	})
}

func TestLUCIContextMethods(t *testing.T) {
	// t.Parallel() manipulates environ
	defer rawctx(`{"hi": {"hello_there": 10}}`)()
	externalContext = extractFromEnv(nil)

	Convey("lctx", t, func() {
		c := context.Background()

		Convey("Get/Set", func() {
			Convey("defaults to env values", func() {
				h := &TestStructure{}
				So(Get(c, "hi", h), ShouldBeNil)
				So(h, ShouldResembleProto, &TestStructure{HelloThere: 10})
			})

			Convey("nop for missing sections", func() {
				h := &TestStructure{}
				So(Get(c, "wut", h), ShouldBeNil)
				So(h, ShouldResembleProto, &TestStructure{})
			})

			Convey("can add section", func() {
				c := Set(c, "wut", &TestStructure{HelloThere: 100})

				h := &TestStructure{}
				So(Get(c, "wut", h), ShouldBeNil)
				So(h, ShouldResembleProto, &TestStructure{HelloThere: 100})

				So(Get(c, "hi", h), ShouldBeNil)
				So(h, ShouldResembleProto, &TestStructure{HelloThere: 10})
			})

			Convey("can override section", func() {
				c := Set(c, "hi", &TestStructure{HelloThere: 100})

				h := &TestStructure{}
				So(Get(c, "hi", h), ShouldBeNil)
				So(h, ShouldResembleProto, &TestStructure{HelloThere: 100})
			})

			Convey("can remove section", func() {
				c := Set(c, "hi", nil)

				h := &TestStructure{}
				So(Get(c, "hi", h), ShouldBeNil)
				So(h, ShouldResembleProto, &TestStructure{HelloThere: 0})
			})

			Convey("allow unknown field", func() {
				blob := json.RawMessage(`{"hello_there":10, "unknown_field": "unknown"}`)
				newLctx := externalContext.clone()
				newLctx.sections["hi"] = &blob
				c := context.WithValue(context.Background(), &lctxKey, newLctx)

				h := &TestStructure{}
				So(Get(c, "hi", h), ShouldBeNil)
				So(h, ShouldResembleProto, &TestStructure{HelloThere: 10})
			})
		})

		Convey("Export", func() {
			Convey("empty export is a noop", func() {
				c = Set(c, "hi", nil)
				e, err := Export(c)
				So(err, ShouldBeNil)
				So(e, ShouldHaveSameTypeAs, &nullExport{})
			})

			Convey("exporting with content gives a live export", func() {
				e, err := Export(c)
				So(err, ShouldBeNil)
				So(e, ShouldHaveSameTypeAs, &liveExport{})
				defer e.Close()
				cmd := exec.Command("something", "something")
				e.SetInCmd(cmd)
				path, ok := environ.New(cmd.Env).Lookup(EnvKey)
				So(ok, ShouldBeTrue)

				// There's a valid JSON there.
				blob, err := os.ReadFile(path)
				So(err, ShouldBeNil)
				So(string(blob), ShouldEqual, `{"hi": {"hello_there": 10}}`)

				// And it is in fact located in same file as was supplied externally.
				So(path, ShouldEqual, externalContext.path)
			})

			Convey("Export reuses files", func() {
				c := Set(c, "blah", &TestStructure{})

				e1, err := Export(c)
				So(err, ShouldBeNil)
				So(e1, ShouldHaveSameTypeAs, &liveExport{})
				defer func() {
					if e1 != nil {
						e1.Close()
					}
				}()

				e2, err := Export(c)
				So(err, ShouldBeNil)
				So(e2, ShouldHaveSameTypeAs, &liveExport{})
				defer func() {
					if e2 != nil {
						e2.Close()
					}
				}()

				// Exact same backing file.
				So(e1.(*liveExport).path, ShouldEqual, e2.(*liveExport).path)

				// It really exists.
				path := e1.(*liveExport).path
				_, err = os.Stat(path)
				So(err, ShouldBeNil)

				// Closing one export keeps the file open.
				e1.Close()
				e1 = nil
				_, err = os.Stat(path)
				So(err, ShouldBeNil)

				// Closing both exports removes the file from disk.
				e2.Close()
				e2 = nil
				_, err = os.Stat(path)
				So(os.IsNotExist(err), ShouldBeTrue)
			})

			Convey("ExportInto creates new files", func() {
				tmp, err := ioutil.TempDir("", "luci_ctx")
				So(err, ShouldBeNil)
				defer func() {
					os.RemoveAll(tmp)
				}()

				c := Set(c, "blah", &TestStructure{})

				e1, err := ExportInto(c, tmp)
				So(err, ShouldBeNil)
				p1 := e1.(*liveExport).path

				e2, err := ExportInto(c, tmp)
				So(err, ShouldBeNil)
				p2 := e2.(*liveExport).path

				// Two different files, both under 'tmp'.
				So(p1, ShouldNotEqual, p2)
				So(filepath.Dir(p1), ShouldEqual, tmp)
				So(filepath.Dir(p2), ShouldEqual, tmp)

				// Both exist.
				_, err = os.Stat(p1)
				So(err, ShouldBeNil)
				_, err = os.Stat(p2)
				So(err, ShouldBeNil)

				// Closing one still keeps the other open.
				So(e1.Close(), ShouldBeNil)
				_, err = os.Stat(p1)
				So(os.IsNotExist(err), ShouldBeTrue)
				_, err = os.Stat(p2)
				So(err, ShouldBeNil)
			})
		})
	})
}
