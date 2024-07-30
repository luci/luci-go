// Copyright 2024 The LUCI Authors.
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

package rewriteimpl

import (
	"bytes"
	"flag"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
)

var train = flag.Bool("test.train", false, "if true, writes .out files instead of checking them.")

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func TestRewriter(t *testing.T) {
	testdata, err := os.ReadDir("testinputs")
	assert.Loosely(t, err, should.BeNil)

	rewriteTest := func(testName string) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()

			dec, fil, err := ParseOne(filepath.Join("testinputs", testName))
			if err != nil {
				t.Fatal(err)
			}
			newFil, rewrote, warned, err := Rewrite(dec, fil, stringset.New(0))
			assert.Loosely(t, err, should.BeNil)

			if warned {
				check.That(t, testName, should.ContainSubstring("_warning"))
			} else {
				check.That(t, testName, should.NotContainSubstring("_warning"))
			}
			if !rewrote {
				check.That(t, testName, should.ContainSubstring("_nochange"))
			} else {
				check.That(t, testName, should.NotContainSubstring("_nochange"))
			}

			// The Name field is, confusingly, the package name.
			// We want this to be 'testoutputs' in the target package.
			newFil.Name.Name = "testoutputs"

			var buf bytes.Buffer
			assert.Loosely(t, format.Node(&buf, dec.Fset, newFil), should.BeNil)

			expectedFile := filepath.Join("testoutputs", testName)
			if strings.Contains(testName, "_nobuild") {
				expectedFile += ".nobuild"
			}

			if *train {
				assert.Loosely(t, os.WriteFile(expectedFile, buf.Bytes(), 0666), should.BeNil)
			} else {
				expected, err := os.ReadFile(expectedFile)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, buf.String(), should.Equal(string(expected)))
			}
		}
	}

	for _, entry := range testdata {
		if strings.HasSuffix(entry.Name(), "_test.go") {
			t.Run(entry.Name(), rewriteTest(entry.Name()))
		}
	}
}
