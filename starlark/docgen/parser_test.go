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

package docgen

import (
	"io/ioutil"
	"path/filepath"
	"testing"
)

func TestParseModule(t *testing.T) {
	const testDataRoot = "testdata/parser"
	infos, err := ioutil.ReadDir(testDataRoot)
	if err != nil {
		t.Fatalf("Failed to ReadDir: %s", err)
	}
	for _, f := range infos {
		name := filepath.Join(testDataRoot, f.Name())
		t.Run(name, func(t *testing.T) { runParserTest(t, name) })
	}
}

func runParserTest(t *testing.T, path string) {
	code, err := ioutil.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to open %q - %s", path, err)
		return
	}
	_, err = ParseModule(filepath.ToSlash(path), string(code))
	if err != nil {
		t.Errorf("%s", err)
	}
}
