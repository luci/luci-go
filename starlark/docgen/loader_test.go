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
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
)

func TestLoader(t *testing.T) {
	const testDataRoot = "testdata/loader"
	infos, err := ioutil.ReadDir(testDataRoot)
	if err != nil {
		t.Fatalf("Failed to ReadDir: %s", err)
	}
	for _, f := range infos {
		name := filepath.Join(testDataRoot, f.Name())
		t.Run(name, func(t *testing.T) { runLoaderTest(t, name) })
	}
}

func runLoaderTest(t *testing.T, path string) {
	loader := Loader{
		Source: func(module string) (string, error) {
			body, err := ioutil.ReadFile(filepath.Join(path, filepath.FromSlash(module)))
			return string(body), err
		},
	}
	syms, err := loader.Load("top.star")
	if err != nil {
		t.Errorf("%s", err)
		return
	}
	for _, sym := range syms {
		fmt.Printf("%s\n", sym)
	}
}
