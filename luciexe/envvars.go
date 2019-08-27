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

package luciexe

import "go.chromium.org/luci/logdog/client/butlerlib/bootstrap"

// TempDirEnvVars is the list of environment variable names which should be set
// to point at the temporary directory for a luciexe.
var TempDirEnvVars = []string{
	// See https://cs.chromium.org/search/?q=MAC_CHROMIUM_TMPDIR+file:file_util_mac
	"MAC_CHROMIUM_TMPDIR",
	"TEMP",
	"TEMPDIR",
	"TMP",
	"TMPDIR",
}

// LogdogNamespaceEnv is the environment variable name for the current Logdog
// namespace. This must hold a fwd-slash-separated logdog StreamName fragment.
//
// It's permissible for this to be empty.
const LogdogNamespaceEnv = bootstrap.EnvNamespace
