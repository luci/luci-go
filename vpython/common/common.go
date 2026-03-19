// Copyright 2022 The LUCI Authors.
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

// Package common contains utilities for forming python commands and searching
// cipd binary for verify.
package common

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// Python returns the python path in python installation.
func Python(path, py string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(path, "bin", py+".exe")
	}
	return filepath.Join(path, "bin", py)
}

// PythonVENV returns the python path in venv.
func PythonVENV(path, py string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(path, "Scripts", py+".exe")
	}
	return filepath.Join(path, "bin", py)
}

// DefaultBundleDir returns the path to the cpython bundle.
// On Darwin, because vpython is an app bundle, cpython is located at
// vpython.app/Contents/Resources. On other platforms cpython is next to the
// vpython binary.
func DefaultBundleDir(version string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join("..", "Resources", version)
	}
	return version
}

// CIPDCommand generates a *exec.Cmd for cipd. It will lookup cipd and its
// wrappers depending on platforms.
func CIPDCommand(arg ...string) *exec.Cmd {
	cipd, err := exec.LookPath("cipd")
	if err != nil {
		cipd = "cipd"
	}

	// Use cmd to execute batch file on windows.
	if filepath.Ext(cipd) == ".bat" {
		return exec.Command("cmd.exe", append([]string{"/C", cipd}, arg...)...)
	}

	return exec.Command(cipd, arg...)
}
