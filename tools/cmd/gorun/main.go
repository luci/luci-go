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

// Package main defines the `gorun` tool, a shorthand tool to extend the
// "go run"-like convenience to packages.
//
// Unfortunately, "go run" is hard to use when the target has more than one
// ".go" file, and even harder when the target has "_test.go" files.
//
// This is just a bootstrap to "go build /path/to/X && /path/to/X" using a
// temporary directory for build storage.
package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/exitcode"
)

func mainImpl(args []string) (int, error) {
	if len(args) < 1 {
		return 1, errors.New("accepts one argument: package")
	}
	pkg, args := args[0], args[1:]

	// Create a temporary output directory to build into.
	tmpdir, err := ioutil.TempDir("", "luci-gorun")
	if err != nil {
		return 1, errors.Annotate(err, "failed to create temporary directory").Err()
	}
	defer os.RemoveAll(tmpdir)

	exePath := filepath.Join(tmpdir, "gorun_target")
	if runtime.GOOS == "windows" {
		exePath += ".exe"
	}

	// Build the package.
	cmd := exec.Command("go", "build", "-o", exePath, pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return 1, errors.Annotate(err, "failed to build: %s", pkg).Err()
	}

	// Run the package.
	cmd = exec.Command(exePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if rc, ok := exitcode.Get(err); ok {
			return rc, nil
		}
		return 1, errors.Annotate(err, "failed to run: %s", pkg).Err()
	}

	return 0, nil
}

func main() {
	rc, err := mainImpl(os.Args[1:])
	if err != nil {
		os.Stderr.WriteString(errors.RenderStack(err))
		os.Stderr.WriteString("\n")
	}
	os.Exit(rc)
}
