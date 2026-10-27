// Copyright 2026 The LUCI Authors.
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

// Package testutil provides shared testing helpers for the luci CLI.
package testutil

import (
	"bytes"
	"io"
	"os"
	"sync"
)

var stdMu sync.Mutex

// CaptureOutput captures stdout and stderr written via os.Stdout and os.Stderr
// while running fn.
func CaptureOutput(fn func()) (stdout, stderr string) {
	stdMu.Lock()
	defer stdMu.Unlock()
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	oldOut := os.Stdout
	oldErr := os.Stderr
	os.Stdout = wOut
	os.Stderr = wErr

	defer func() {
		os.Stdout = oldOut
		os.Stderr = oldErr
	}()

	outC := make(chan string, 1)
	errC := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rOut)
		outC <- buf.String()
	}()
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rErr)
		errC <- buf.String()
	}()

	func() {
		defer func() {
			_ = wOut.Close()
			_ = wErr.Close()
		}()
		fn()
	}()

	return <-outC, <-errC
}

// CaptureStdout captures stdout written via os.Stdout while running fn.
func CaptureStdout(fn func()) string {
	out, _ := CaptureOutput(fn)
	return out
}

// CaptureStderr captures stderr written via os.Stderr while running fn.
func CaptureStderr(fn func()) string {
	_, err := CaptureOutput(fn)
	return err
}
