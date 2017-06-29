// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/system/exitcode"
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
		for _, line := range errors.RenderStack(err) {
			os.Stderr.WriteString(line + "\n")
		}
	}
	os.Exit(rc)
}
