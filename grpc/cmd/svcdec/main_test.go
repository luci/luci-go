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

package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var (
	inputDir   = filepath.Join("..", "..", "internal", "svctool", "testdata")
	goldenFile = "testdata/s1server_dec.golden"
)

func TestMain(t *testing.T) {
	t.Parallel()

	ftt.Run("svcdec", t, func(t *ftt.Test) {
		tmpDir, err := ioutil.TempDir("", "")
		assert.Loosely(t, err, should.BeNil)
		defer os.RemoveAll(tmpDir)

		run := func(args ...string) error {
			t := tool()
			t.ParseArgs(args)
			return t.Run(context.Background(), generate)
		}

		t.Run("Works", func(t *ftt.Test) {
			output := filepath.Join(tmpDir, "s1server_dec.go")
			err := run(
				"-output", output,
				"-type", "S1Server,S2Server",
				inputDir,
			)
			assert.Loosely(t, err, should.BeNil)

			want, err := os.ReadFile(goldenFile)
			assert.Loosely(t, err, should.BeNil)

			got, err := os.ReadFile(output)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, string(got), should.Equal(string(want)))
		})
	})
}
