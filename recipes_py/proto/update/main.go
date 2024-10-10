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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
)

func main() {
	ctx := gologger.StdConfig.Use(logging.SetLevel(context.Background(), logging.Info))
	if err := mainImpl(ctx); err != nil {
		logging.Errorf(ctx, "%s\n", err)
		os.Exit(1)
	}
}

func mainImpl(ctx context.Context) error {
	logging.Infof(ctx, "downloading recipes_cfg.proto from recipes-py repo")
	gitilesClient, err := gitiles.NewRESTClient(&http.Client{}, "chromium.googlesource.com", false)
	if err != nil {
		return fmt.Errorf("failed to create gitiles client: %w", err)
	}
	resp, err := gitilesClient.DownloadFile(ctx, &gitilespb.DownloadFileRequest{
		Project:    "infra/luci/recipes-py",
		Committish: "HEAD",
		Path:       "recipe_engine/recipes_cfg.proto",
		Format:     gitilespb.DownloadFileRequest_TEXT,
	})
	if err != nil {
		return fmt.Errorf("failed to download recipes_cfg.proto: %w", err)
	}
	logging.Infof(ctx, "recipes_cfg.proto downloaded. Updating recipes_cfg.proto on disk")

	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	repoRoot, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to find the repo root: %w", err)
	}

	recipes_proto_dir := filepath.Join(strings.TrimSpace(string(repoRoot)), "recipes_py", "proto")
	outFile, err := os.Create(filepath.Join(recipes_proto_dir, "recipes_cfg.proto"))
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	fmt.Fprintf(outFile, "// DO NOT EDIT!!!\n")
	fmt.Fprintf(outFile, "// Copied from https://chromium.googlesource.com/infra/luci/recipes-py/+/HEAD/recipe_engine/recipes_cfg.proto\n")
	fmt.Fprintf(outFile, "// Please run the update script (update/main.go) to pull the latest proto.\n\n")

	var packageLineInserted bool
	for _, line := range strings.Split(strings.TrimSpace(resp.GetContents()), "\n") {
		fmt.Fprintln(outFile, line)
		if !packageLineInserted && line == "package recipe_engine;" {
			fmt.Fprintf(outFile, "\noption go_package = \"go.chromium.org/luci/recipes_py/proto;recipespb\";\n")
			packageLineInserted = true
		}
	}
	logging.Infof(ctx, "successfully updated recipes_cfg.proto. Regenerating proto...")

	cmd = exec.Command("go", "generate")
	cmd.Dir = recipes_proto_dir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to regenerate the proto: %w", err)
	}
	logging.Infof(ctx, "regeneration completed.")
	return nil
}
