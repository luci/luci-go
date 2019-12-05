// Copyright 2019 The Fuchsia Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/luciexe/exe"
	"go.chromium.org/luci/luciexe/invoke"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// Root dir to ensure recipe bundle CIPD package.
	cipdRoot = "rbundle"

	// Top-level step name for a bootstrapped recipe execution.
	namespace = "bootstrapped_recipe"

	// Recipe bundle CIPD package name to use.
	recipePackageName = "fuchsia/infra/recipe_bundles/fuchsia.googlesource.com/infra/recipes"

	// Pinned recipes version.
	recipeVersion = "git_revision:de6d60865578802e816f017eacde97b310a7d5ff"
)

// Ensure recipe bundle from CIPD at a specified dir for a specified version.
func ensure(ctx context.Context, pkgName string, pkgVersion string, installDir string) error {
	ef := fmt.Sprintf("%s %s", pkgName, pkgVersion)
	cmd := exec.CommandContext(ctx, "cipd", "ensure", "-root", installDir, "-ensure-file", "-")
	cmd.Stdin = strings.NewReader(ef)
	return cmd.Run()
}

func main() {
	exe.Run(func(ctx context.Context, build *buildbucketpb.Build, send exe.BuildSender) error {
		err := ensure(ctx, recipePackageName, recipeVersion, cipdRoot)
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		invokeOpts := invoke.Options{Namespace: namespace}
		exePath := filepath.Join(cwd, cipdRoot, "luciexe")
		subprocess, err := invoke.Start(ctx, exePath, build, &invokeOpts)
		if err != nil {
			return err
		}
		build.Steps = append(build.Steps, subprocess.Step)
		send()
		_, err = subprocess.Wait()
		return err
	})
}
