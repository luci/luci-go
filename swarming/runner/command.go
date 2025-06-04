// Copyright 2021 The LUCI Authors.
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

package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

const (
	executableSuffixParameter = "${EXECUTABLE_SUFFIX}"
	isolatedOutdirParameter   = "${ISOLATED_OUTDIR}"
	swarmingBotFileParameter  = "${SWARMING_BOT_FILE}"
)

// ReplaceCommandParameters replaces parameter tokens with appropriate values in a
// string.
func ReplaceCommandParameters(ctx context.Context, arg, outDir, botFile string) (string, error) {

	if runtime.GOOS == "windows" {
		arg = strings.Replace(arg, executableSuffixParameter, ".exe", -1)
	} else {
		arg = strings.Replace(arg, executableSuffixParameter, "", -1)
	}
	replaceSlash := false

	if strings.Contains(arg, isolatedOutdirParameter) {
		if outDir == "" {
			return "", errors.New("output directory is requested in command or env var, but not provided; please specify one")
		}
		arg = strings.Replace(arg, isolatedOutdirParameter, outDir, -1)
		replaceSlash = true
	}

	if strings.Contains(arg, swarmingBotFileParameter) {
		if botFile != "" {
			arg = strings.Replace(arg, swarmingBotFileParameter, botFile, -1)
			replaceSlash = true
		} else {
			logging.Warningf(ctx, "swarmingBotFileParameter found in command or env var, but no bot_file specified. Leaving parameter unchanged.")
		}
	}

	if replaceSlash {
		arg = strings.Replace(arg, "/", string(filepath.Separator), -1)
	}

	return arg, nil
}

// ProcessCommand replaces parameters in a command line.
func ProcessCommand(ctx context.Context, command []string, outDir, botFile string) ([]string, error) {
	newCommand := make([]string, 0, len(command))
	for _, arg := range command {
		newArg, err := ReplaceCommandParameters(ctx, arg, outDir, botFile)
		if err != nil {
			return nil, fmt.Errorf("failed to replace parameter %s: %v", arg, err)
		}
		newCommand = append(newCommand, newArg)
	}
	return newCommand, nil
}
