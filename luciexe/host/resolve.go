// Copyright 2024 The LUCI Authors.
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

package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
)

// ResolveExeCmd resolves the given host options and returns the command for
// luciexe host to invoke as a luciexe.
//
// This includes resolving paths relative to the current working directory, or
// the directory contains agent inputs if DownloadAgentInputs enabled.
func ResolveExeCmd(opts *Options, defaultPayloadPath string) ([]string, error) {
	exeArgs := make([]string, 0, len(opts.BaseBuild.Exe.Wrapper)+len(opts.BaseBuild.Exe.Cmd)+1)

	if len(opts.BaseBuild.Exe.Wrapper) != 0 {
		exeArgs = append(exeArgs, opts.BaseBuild.Exe.Wrapper...)
		exeArgs = append(exeArgs, "--")

		if strings.Contains(exeArgs[0], "/") || strings.Contains(exeArgs[0], "\\") {
			absPath, err := filepath.Abs(exeArgs[0])
			if err != nil {
				return nil, errors.Fmt("absoluting wrapper path: %q: %w", exeArgs[0], err)
			}
			exeArgs[0] = absPath
		}

		cmdPath, err := exec.LookPath(exeArgs[0])
		if err != nil {
			return nil, errors.Fmt("wrapper not found: %q: %w", exeArgs[0], err)
		}
		exeArgs[0] = cmdPath
	}

	exeCmd := opts.BaseBuild.Exe.Cmd[0]
	payloadPath := defaultPayloadPath
	for p, purpose := range opts.BaseBuild.GetInfra().GetBuildbucket().GetAgent().GetPurposes() {
		if purpose == bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD {
			payloadPath = p
			break
		}
	}

	if !filepath.IsAbs(payloadPath) && opts.DownloadAgentInputs {
		payloadPath = filepath.Join(opts.agentInputsDir, payloadPath)
	}
	exePath, err := processCmd(payloadPath, exeCmd)
	if err != nil {
		return nil, err
	}
	exeArgs = append(exeArgs, exePath)
	exeArgs = append(exeArgs, opts.BaseBuild.Exe.Cmd[1:]...)

	return exeArgs, nil
}

func resolveExe(path string) (string, error) {
	if filepath.Ext(path) != "" {
		return path, nil
	}

	lme := errors.NewLazyMultiError(2)
	for i, ext := range []string{".exe", ".bat"} {
		candidate := path + ext
		if _, err := os.Stat(candidate); !lme.Assign(i, err) {
			return candidate, nil
		}
	}

	me := lme.Get().(errors.MultiError)
	return path, errors.Fmt("cannot find .exe (%q) or .bat (%q)", me[0], me[1])
}

// processCmd resolves the cmd by constructing the absolute path and resolving
// the exe suffix.
func processCmd(path, cmd string) (string, error) {
	relPath := filepath.Join(path, cmd)
	absPath, err := filepath.Abs(relPath)
	if err != nil {
		return "", errors.Fmt("absoluting %q: %w", relPath, err)
	}
	if runtime.GOOS == "windows" {
		absPath, err = resolveExe(absPath)
		if err != nil {
			return "", errors.Fmt("resolving %q: %w", absPath, err)
		}
	}
	return absPath, nil
}
