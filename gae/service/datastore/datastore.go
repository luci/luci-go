// Copyright 2015 The LUCI Authors.
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

package datastore

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v2"
)

// ParseIndexYAML parses the contents of a index YAML file into a list of
// IndexDefinitions.
func ParseIndexYAML(content io.Reader) ([]*IndexDefinition, error) {
	serialized, err := ioutil.ReadAll(content)
	if err != nil {
		return nil, err
	}

	var m map[string][]*IndexDefinition
	if err := yaml.Unmarshal(serialized, &m); err != nil {
		return nil, err
	}

	if _, ok := m["indexes"]; !ok {
		return nil, fmt.Errorf("datastore: missing key `indexes`: %v", m)
	}
	return m["indexes"], nil
}

// getCallingTestFilePath looks up the call stack until the specified
// maxStackDepth and returns the absolute path of the first source filename
// ending with `_test.go`. If no test file is found, getCallingTestFilePath
// returns a non-nil error.
func getCallingTestFilePath(maxStackDepth int) (string, error) {
	pcs := make([]uintptr, maxStackDepth)

	for _, pc := range pcs[:runtime.Callers(0, pcs)] {
		path, _ := runtime.FuncForPC(pc - 1).FileLine(pc - 1)
		if filename := filepath.Base(path); strings.HasSuffix(filename, "_test.go") {
			return path, nil
		}
	}

	return "", fmt.Errorf("datastore: failed to determine source file name")
}

// FindAndParseIndexYAML walks up from the directory specified by path until it
// finds a `index.yaml` or `index.yml` file. If an index YAML file
// is found, it opens and parses the file, and returns all the indexes found.
// If path is a relative path, it is converted into an absolute path
// relative to the calling test file. To determine the path of the calling test
// file, FindAndParseIndexYAML walks upto a maximum of 100 call stack frames
// looking for a file ending with `_test.go`.
//
// FindAndParseIndexYAML returns a non-nil error if the root of the drive is
// reached without finding an index YAML file, if there was
// an error reading the found index YAML file, or if the calling test file could
// not be located in the case of a relative path argument.
func FindAndParseIndexYAML(path string) ([]*IndexDefinition, error) {
	var currentDir string

	if filepath.IsAbs(path) {
		currentDir = path
	} else {
		testPath, err := getCallingTestFilePath(100)
		if err != nil {
			return nil, err
		}
		currentDir = filepath.Join(filepath.Dir(testPath), path)
	}

	isRoot := func(dir string) bool {
		parentDir := filepath.Dir(dir)
		return os.IsPathSeparator(dir[len(dir)-1]) && os.IsPathSeparator(parentDir[len(parentDir)-1])
	}

	for {
		for _, filename := range []string{"index.yml", "index.yaml"} {
			file, err := os.Open(filepath.Join(currentDir, filename))
			if err == nil {
				defer file.Close()
				return ParseIndexYAML(file)
			}
		}

		if isRoot(currentDir) {
			return nil, fmt.Errorf("datastore: failed to find index YAML file")
		}

		currentDir = filepath.Dir(currentDir)
	}
}
