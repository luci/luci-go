// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/cipkg/core"
)

// ActionCIPDExportTransformer is the default transformer for
// core.ActionCIPDExport.
func ActionCIPDExportTransformer(a *core.ActionCIPDExport, deps []Package) (*core.Derivation, error) {
	return ReexecDerivation(a, true)
}

// ActionCIPDExportExecutor is the default executor for core.ActionCIPDExport.
// It will use the cipd binary/script (or cipd.bat/cipd.exe on windows) on PATH
// to export the packages required.
func ActionCIPDExportExecutor(ctx context.Context, a *core.ActionCIPDExport, out string) error {
	env := environ.FromCtx(ctx)
	env.Update(environ.New(a.Env))
	cmd := cipdCommand(ctx, "export", "--root", out, "--ensure-file", "-")
	cmd.Env = env.Sorted()
	cmd.Stdin = strings.NewReader(a.EnsureFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cipdCommand(ctx context.Context, arg ...string) *exec.Cmd {
	cipd := lookup("cipd")

	// Use cmd to execute batch file on windows.
	if filepath.Ext(cipd) == ".bat" {
		return exec.CommandContext(ctx, lookup("cmd.exe"), append([]string{"/C", cipd}, arg...)...)
	}

	return exec.CommandContext(ctx, cipd, arg...)
}

func lookup(bin string) string {
	if path, err := exec.LookPath(bin); err == nil {
		return path
	}
	return bin
}
