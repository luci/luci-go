// Copyright 2017 The LUCI Authors.
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

package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/errors"
)

// LockFileEnvVariable specifies the directory of the lock file.
const LockFileEnvVariable = "MMUTEX_LOCK_DIR"

// LockFileName specifies the name of the lock file within $MMUTEX_LOCK_DIR.
const LockFileName = "mmutex.lock"

var fslockTimeout = 2 * time.Hour
var fslockPollingInterval = 5 * time.Second

// Returns the lock file path based on the environment variable, or an empty string if no
// lock file should be used.
func computeLockFilePath(env subcommands.Env) (string, error) {
	envVar := env[LockFileEnvVariable]
	if !envVar.Exists {
		return "", nil
	}

	lockFileDir := envVar.Value
	if !filepath.IsAbs(lockFileDir) {
		return "", errors.Reason("Lock file directory %s must be an absolute path", lockFileDir).Err()
	}

	if _, err := os.Stat(lockFileDir); os.IsNotExist(err) {
		fmt.Printf("Lock file directory %s does not exist, mmutex acting as a passthrough.", lockFileDir)
		return "", nil
	}

	return filepath.Join(lockFileDir, LockFileName), nil
}
