// Copyright 2023 The LUCI Authors.
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

package exec

import (
	"os/exec"
)

// LookPath searches for an executable named file in the directories named by
// the PATH environment variable. If file contains a slash, it is tried directly
// and the PATH is not consulted. The result may be an absolute path or a path
// relative to the current directory.
//
// This is an alias of `"os/exec".LookPath`.
var LookPath = exec.LookPath

// Error is returned by LookPath when it fails to classify a file as an
// executable.
//
// This is an alias of `"os/exec".Error`.
type Error = exec.Error

// An ExitError reports an unsuccessful exit by a command.
//
// This is an alias of `"os/exec".ExitError`.
type ExitError = exec.ExitError
