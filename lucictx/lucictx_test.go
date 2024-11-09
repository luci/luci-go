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

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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

	ftt.Run("extractFromEnv", t, func(t *ftt.Test) {
		os.Unsetenv(EnvKey)
		buf := &bytes.Buffer{}

		t.Run("works with missing envvar", func(t *ftt.Test) {
			assert.Loosely(t, extractFromEnv(buf), should.Resemble(&lctx{}))
			assert.Loosely(t, buf.String(), should.BeEmpty)
			_, ok := os.LookupEnv(EnvKey)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("works with bad envvar", func(t *ftt.Test) {
			os.Setenv(EnvKey, "sup")
			assert.Loosely(t, extractFromEnv(buf), should.Resemble(&lctx{}))
			assert.Loosely(t, buf.String(), should.ContainSubstring("Could not open LUCI_CONTEXT file"))
		})

		t.Run("works with bad file", func(t *ftt.Test) {
			defer rawctx(`"not a map"`)()

			assert.Loosely(t, extractFromEnv(buf), should.Resemble(&lctx{}))
			assert.Loosely(t, buf.String(), should.ContainSubstring("cannot unmarshal string into Go value"))
		})

		t.Run("ignores bad sections", func(t *ftt.Test) {
			defer rawctx(`{"hi": {"hello_there": 10}, "not": "good"}`)()

			blob := json.RawMessage(`{"hello_there":10}`)
			assert.Loosely(t, extractFromEnv(buf).sections, should.Resemble(map[string]*json.RawMessage{
				"hi": &blob,
			}))
			assert.Loosely(t, buf.String(), should.ContainSubstring(`section "not": Not a map`))
		})
	})
}

func TestLUCIContextMethods(t *testing.T) {
	// t.Parallel() manipulates environ
	defer rawctx(`{"hi": {"hello_there": 10}}`)()
	externalContext = extractFromEnv(nil)

	ftt.Run("lctx", t, func(t *ftt.Test) {
		c := context.Background()

		t.Run("Get/Set", func(t *ftt.Test) {
			t.Run("defaults to env values", func(t *ftt.Test) {
				h := &TestStructure{}
				assert.Loosely(t, Get(c, "hi", h), should.BeNil)
				assert.Loosely(t, h, should.Resemble(&TestStructure{HelloThere: 10}))
			})

			t.Run("nop for missing sections", func(t *ftt.Test) {
				h := &TestStructure{}
				assert.Loosely(t, Get(c, "wut", h), should.BeNil)
				assert.Loosely(t, h, should.Resemble(&TestStructure{}))
			})

			t.Run("can add section", func(t *ftt.Test) {
				c := Set(c, "wut", &TestStructure{HelloThere: 100})

				h := &TestStructure{}
				assert.Loosely(t, Get(c, "wut", h), should.BeNil)
				assert.Loosely(t, h, should.Resemble(&TestStructure{HelloThere: 100}))

				assert.Loosely(t, Get(c, "hi", h), should.BeNil)
				assert.Loosely(t, h, should.Resemble(&TestStructure{HelloThere: 10}))
			})

			t.Run("can override section", func(t *ftt.Test) {
				c := Set(c, "hi", &TestStructure{HelloThere: 100})

				h := &TestStructure{}
				assert.Loosely(t, Get(c, "hi", h), should.BeNil)
				assert.Loosely(t, h, should.Resemble(&TestStructure{HelloThere: 100}))
			})

			t.Run("can remove section", func(t *ftt.Test) {
				c := Set(c, "hi", nil)

				h := &TestStructure{}
				assert.Loosely(t, Get(c, "hi", h), should.BeNil)
				assert.Loosely(t, h, should.Resemble(&TestStructure{HelloThere: 0}))
			})

			t.Run("allow unknown field", func(t *ftt.Test) {
				blob := json.RawMessage(`{"hello_there":10, "unknown_field": "unknown"}`)
				newLctx := externalContext.clone()
				newLctx.sections["hi"] = &blob
				c := context.WithValue(context.Background(), &lctxKey, newLctx)

				h := &TestStructure{}
				assert.Loosely(t, Get(c, "hi", h), should.BeNil)
				assert.Loosely(t, h, should.Resemble(&TestStructure{HelloThere: 10}))
			})
		})

		t.Run("Export", func(t *ftt.Test) {
			t.Run("empty export is a noop", func(t *ftt.Test) {
				c = Set(c, "hi", nil)
				e, err := Export(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, e, should.HaveType[*nullExport])
			})

			t.Run("exporting with content gives a live export", func(t *ftt.Test) {
				e, err := Export(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, e, should.HaveType[*liveExport])
				defer e.Close()
				cmd := exec.Command("something", "something")
				e.SetInCmd(cmd)
				path, ok := environ.New(cmd.Env).Lookup(EnvKey)
				assert.Loosely(t, ok, should.BeTrue)

				// There's a valid JSON there.
				blob, err := os.ReadFile(path)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(blob), should.Equal(`{"hi": {"hello_there": 10}}`))

				// And it is in fact located in same file as was supplied externally.
				assert.Loosely(t, path, should.Equal(externalContext.path))
			})

			t.Run("Export reuses files", func(t *ftt.Test) {
				c := Set(c, "blah", &TestStructure{})

				e1, err := Export(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, e1, should.HaveType[*liveExport])
				defer func() {
					if e1 != nil {
						e1.Close()
					}
				}()

				e2, err := Export(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, e2, should.HaveType[*liveExport])
				defer func() {
					if e2 != nil {
						e2.Close()
					}
				}()

				// Exact same backing file.
				assert.Loosely(t, e1.(*liveExport).path, should.Equal(e2.(*liveExport).path))

				// It really exists.
				path := e1.(*liveExport).path
				_, err = os.Stat(path)
				assert.Loosely(t, err, should.BeNil)

				// Closing one export keeps the file open.
				e1.Close()
				e1 = nil
				_, err = os.Stat(path)
				assert.Loosely(t, err, should.BeNil)

				// Closing both exports removes the file from disk.
				e2.Close()
				e2 = nil
				_, err = os.Stat(path)
				assert.Loosely(t, os.IsNotExist(err), should.BeTrue)
			})

			t.Run("ExportInto creates new files", func(t *ftt.Test) {
				tmp, err := ioutil.TempDir("", "luci_ctx")
				assert.Loosely(t, err, should.BeNil)
				defer func() {
					os.RemoveAll(tmp)
				}()

				c := Set(c, "blah", &TestStructure{})

				e1, err := ExportInto(c, tmp)
				assert.Loosely(t, err, should.BeNil)
				p1 := e1.(*liveExport).path

				e2, err := ExportInto(c, tmp)
				assert.Loosely(t, err, should.BeNil)
				p2 := e2.(*liveExport).path

				// Two different files, both under 'tmp'.
				assert.Loosely(t, p1, should.NotEqual(p2))
				assert.Loosely(t, filepath.Dir(p1), should.Equal(tmp))
				assert.Loosely(t, filepath.Dir(p2), should.Equal(tmp))

				// Both exist.
				_, err = os.Stat(p1)
				assert.Loosely(t, err, should.BeNil)
				_, err = os.Stat(p2)
				assert.Loosely(t, err, should.BeNil)

				// Closing one still keeps the other open.
				assert.Loosely(t, e1.Close(), should.BeNil)
				_, err = os.Stat(p1)
				assert.Loosely(t, os.IsNotExist(err), should.BeTrue)
				_, err = os.Stat(p2)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}
