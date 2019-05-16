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

package isolated

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"go.chromium.org/luci/common/logging"
)

const (
	executableSuffixParameter = "${EXECUTABLE_SUFFIX}"
	isolatedOutdirParameter   = "${ISOLATED_OUTDIR}"
	swarmingBotFileParameter  = "${SWARMING_BOT_FILE}"
)

// replaceParameters replaces parameter tokens with appropriate values in a
// string.
func replaceParameters(ctx context.Context, arg, outDir, botFile string) (string, error) {

	if runtime.GOOS == "windows" {
		arg = strings.ReplaceAll(arg, executableSuffixParameter, ".exe")
	} else {
		arg = strings.ReplaceAll(arg, executableSuffixParameter, "")
	}
	replaceSlash := false

	if strings.Contains(arg, isolatedOutdirParameter) {
		if outDir == "" {
			return "", errors.New("output directory is requested in command or env var, but not provided; please specify one")
		}
		arg = strings.ReplaceAll(arg, isolatedOutdirParameter, outDir)
		replaceSlash = true
	}

	if strings.Contains(arg, swarmingBotFileParameter) {
		if botFile != "" {
			arg = strings.ReplaceAll(arg, swarmingBotFileParameter, botFile)
			replaceSlash = true
		} else {
			logging.Warningf(ctx, "swarmingBotFileParameter found in command or env var, but no bot_file specified. Leaving parameter unchanged.")
		}
	}

	if replaceSlash {
		arg = strings.ReplaceAll(arg, "/", string(filepath.Separator))
	}

	return arg, nil
}

// processCommand replaces parameters in a command line.
func processCommand(ctx context.Context, command []string, outDir, botFile string) ([]string, error) {
	newCommand := make([]string, 0, len(command))
	for _, arg := range command {
		newArg, err := replaceParameters(ctx, arg, outDir, botFile)
		if err != nil {
			return nil, fmt.Errorf("failed to replace parameter %s: %v", arg, err)
		}
		newCommand = append(newCommand, newArg)
	}
	return newCommand, nil
}
