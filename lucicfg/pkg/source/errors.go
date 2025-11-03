// Copyright 2025 The LUCI Authors.
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

package source

import (
	"bytes"
	"errors"
	"fmt"
)

var (
	ErrMissingCommit       = errors.New("commit is missing")
	ErrMissingObject       = errors.New("object is missing")
	ErrObjectNotPrefetched = errors.New("object was not prefeched")
)

// GitError indicate a failed git execution (often with unknown root cause).
type GitError struct {
	Err     error  // the wrapped error from the exec.Command or context.Context
	CmdLine string // the command line with failed call
	Output  []byte // the capture output if it was captured (either stdout or combined)
}

// Error implements error interface.
func (e *GitError) Error() string {
	if trimmed := bytes.TrimSpace(e.Output); len(trimmed) > 0 {
		return fmt.Sprintf("running %s: %s:\n%s", e.CmdLine, e.Err, trimmed)
	}
	return fmt.Sprintf("running %s: %s", e.CmdLine, e.Err)
}

// Unwrap allows to traverse this error.
func (e *GitError) Unwrap() error {
	return e.Err
}
