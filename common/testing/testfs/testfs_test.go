// Copyright 2019 The LUCI Authors.
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

package testfs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSimple(t *testing.T) {
	t.Parallel()

	MustWithTempDir(t, "", func(dir string) {
		layout := map[string]string{
			"a/b":    "ab",
			"a/c/d":  "acd",
			"e":      "e",
			"f/":     "",
			"g/h/i/": "",
			"j/k/l":  "jkl",
		}

		if err := Build(dir, layout); err != nil {
			t.Errorf("failed to call Build: %v", err)
		}

		got, err := Collect(dir)
		if err != nil {
			t.Errorf("failed to call Collect: %v", err)
		}

		if !cmp.Equal(got, layout) {
			t.Errorf("got: %v\nwant: %v", got, layout)
		}
	})()
}
