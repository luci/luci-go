// Copyright 2020 The LUCI Authors.
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

package ledcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/led/job"
)

// EditRecipeBundleOpts are user-provided options for the recipe bundling
// process.
type EditRecipeBundleOpts struct {
	// Path on disk to the repo to extract the recipes from. May be a subdirectory
	// of the repo, as long as `git rev-parse --show-toplevel` can find the root
	// of the repository.
	//
	// If empty, uses the current working directory.
	RepoDir string

	// Overrides is a mapping of recipe project id (e.g. "recipe_engine") to
	// a local path to a checkout of that repo (e.g. "/path/to/recipes-py.git").
	//
	// When the bundle is created, this local repo will be used instead of the
	// pinned version of this recipe project id. This is helpful for preparing
	// bundles which have code changes in multiple recipe repos.
	Overrides map[string]string

	// DebugSleep is the amount of time to wait after the recipe completes
	// execution (either success or failure). This is injected into the generated
	// recipe bundle as a 'sleep X' command after the invocation of the recipe
	// itself.
	DebugSleep time.Duration

	// PropertyOnly determines whether to pass the recipe bundle's CAS reference
	// as a property and preserve the executable and payload of the input job
	// rather than overwriting it.
	PropertyOnly bool
}

const (
	// In PropertyOnly mode or if the "led_builder_is_bootstrapped" property
	// of the build is true, this property will be set with the CAS digest
	// of the executable of the recipe bundle.
	CASRecipeBundleProperty = "led_cas_recipe_bundle"
)

// EditRecipeBundle overrides the recipe bundle in the given job with one
// located on disk.
//
// It isolates the recipes from the repository in the given working directory
// into the UserPayload under the directory "kitchen-checkout/". If there's an
// existing directory in the UserPayload at that location, it will be removed.
func EditRecipeBundle(ctx context.Context, authOpts auth.Options, jd *job.Definition, opts *EditRecipeBundleOpts) error {
	if jd.GetBuildbucket() == nil {
		return errors.New("ledcmd.EditRecipeBundle is only available for Buildbucket tasks")
	}

	if opts == nil {
		opts = &EditRecipeBundleOpts{}
	}

	recipesPy, err := findRecipesPy(ctx, opts.RepoDir)
	if err != nil {
		return err
	}
	logging.Debugf(ctx, "using recipes.py: %q", recipesPy)

	extraProperties := make(map[string]string)
	setRecipeBundleProperty := opts.PropertyOnly || jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties().GetFields()[job.LEDBuilderIsBootstrappedProperty].GetBoolValue()
	if setRecipeBundleProperty {
		// In property-only mode, we want to leave the original payload as is
		// and just upload the recipe bundle as a brand new independent CAS
		// archive for the job's executable to download.
		bundlePath, err := os.MkdirTemp("", "led-recipe-bundle")
		if err != nil {
			return errors.Fmt("creating temporary recipe bundle directory: %w", err)
		}
		if err := opts.prepBundle(ctx, opts.RepoDir, recipesPy, bundlePath); err != nil {
			return err
		}
		logging.Infof(ctx, "isolating recipes")
		casClient, err := newCASClient(ctx, authOpts, jd)
		if err != nil {
			return err
		}
		casRef, err := uploadToCas(ctx, casClient, bundlePath)
		if err != nil {
			return err
		}
		m := &protojson.MarshalOptions{
			UseProtoNames: true,
		}
		jsonCASRef, err := m.Marshal(casRef)
		if err != nil {
			return errors.Fmt("encoding CAS user payload: %w", err)
		}
		extraProperties[CASRecipeBundleProperty] = string(jsonCASRef)
	} else {
		if err := EditIsolated(ctx, authOpts, jd, func(ctx context.Context, dir string) error {
			bundlePath := dir
			if !jd.GetBuildbucket().BbagentDownloadCIPDPkgs() {
				bundlePath = filepath.Join(dir, job.RecipeDirectory)
			}
			// Remove existing bundled recipes, if any. Ignore the error.
			os.RemoveAll(bundlePath)
			if err := opts.prepBundle(ctx, opts.RepoDir, recipesPy, bundlePath); err != nil {
				return err
			}
			logging.Infof(ctx, "isolating recipes")
			return nil
		}); err != nil {
			return err
		}
	}

	return jd.HighLevelEdit(func(je job.HighLevelEditor) {
		if setRecipeBundleProperty {
			je.Properties(extraProperties, false)
		}
		if opts.DebugSleep != 0 {
			je.Env(map[string]string{
				"RECIPES_DEBUG_SLEEP": fmt.Sprintf("%f", opts.DebugSleep.Seconds()),
			})
		}
	})
}

func logCmd(ctx context.Context, inDir string, arg0 string, args ...string) *exec.Cmd {
	ret := exec.CommandContext(ctx, arg0, args...)
	ret.Dir = inDir
	logging.Debugf(ctx, "Running (from %q) - %s %v", inDir, arg0, args)
	return ret
}

func cmdErr(cmd *exec.Cmd, err error, reason string) error {
	if err == nil {
		return nil
	}
	var outErr string
	if ee, ok := err.(*exec.ExitError); ok {
		outErr = strings.TrimSpace(string(ee.Stderr))
		if len(outErr) > 128 {
			outErr = outErr[:128] + "..."
		}
	} else {
		outErr = err.Error()
	}
	return errors.Fmt("running %q: %s: %s: %w", strings.Join(cmd.Args, " "), reason, outErr, err)
}

func appendText(path, fmtStr string, items ...any) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, fmtStr, items...)
	return err
}

func (opts *EditRecipeBundleOpts) prepBundle(ctx context.Context, inDir, recipesPy, toDirectory string) error {
	logging.Infof(ctx, "bundling recipes")
	args := []string{
		recipesPy,
	}
	if logging.GetLevel(ctx) < logging.Info {
		args = append(args, "-v")
	}
	for projID, path := range opts.Overrides {
		args = append(args, "-O", fmt.Sprintf("%s=%s", projID, path))
	}
	args = append(args, "bundle", "--destination", filepath.Join(toDirectory))

	// Always prefer python3 to python
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		return errors.Fmt("unable to find python3 or python in $PATH: %w", err)
	}

	cmd := logCmd(ctx, inDir, python, args...)
	if logging.GetLevel(ctx) < logging.Info {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmdErr(cmd, cmd.Run(), "creating bundle")
}

// findRecipesPy locates the current repo's `recipes.py`. It does this by:
//   - call deduceRepoRoot to get the repo root.
//   - loading the recipes.cfg at infra/config/recipes.cfg
//   - stat'ing the recipes.py implied by the recipes_path in that cfg file.
//
// Failure will return an error.
//
// On success, the absolute path to recipes.py is returned.
func findRecipesPy(ctx context.Context, inDir string) (string, error) {
	repoRoot, err := deduceRepoRoot(ctx, inDir)
	if err != nil {
		return "", err
	}

	pth := filepath.Join(repoRoot, "infra", "config", "recipes.cfg")
	switch st, err := os.Stat(pth); {
	case err != nil:
		return "", errors.Fmt("reading recipes.cfg: %w", err)
	case !st.Mode().IsRegular():
		return "", errors.Fmt("%q is not a regular file", pth)
	}

	type recipesJSON struct {
		RecipesPath string `json:"recipes_path"`
	}
	rj := &recipesJSON{}

	f, err := os.Open(pth)
	if err != nil {
		return "", errors.Fmt("reading recipes.cfg: %q", pth)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(rj); err != nil {
		return "", errors.Fmt("parsing recipes.cfg: %q", pth)
	}

	return filepath.Join(
		repoRoot, filepath.FromSlash(rj.RecipesPath), "recipes.py"), nil
}

var errRepoRootNotFound = errors.New("can not determine repo root. Is this a recipe repo?")

// deduceRepoRoot returns the most possible root repo root for the `indir`.
//
// It first consults `git` to find the repo root. If it doesn't work, it will
// walk up the directories tree to find the directory that contains
// `infra/config/recipes.cfg`. If either attempts fail, `errRepoRootNotFound`
// will be returned.
func deduceRepoRoot(ctx context.Context, inDir string) (string, error) {
	cmd := logCmd(ctx, inDir, "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	switch exitErr := (&exec.ExitError{}); {
	case err == nil:
		return strings.TrimSpace(string(out)), nil
	case !errors.As(err, &exitErr):
		return "", cmdErr(cmd, err, "finding git repo")
	}

	curPath, err := filepath.Abs(inDir)
	if err != nil {
		return "", errors.Fmt("compute absolute path: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(curPath, "infra", "config", "recipes.cfg")); err == nil {
			return curPath, nil
		}
		// prevent walking across known source control boundary. `.git` here is just
		// for visibility purpose cause the earlier `git rev-parse`` command would
		// have succeeded.
		for _, sentinel := range []string{".git", ".citc"} {
			if _, err := os.Stat(filepath.Join(curPath, sentinel)); err == nil {
				return "", errRepoRootNotFound
			}
		}
		parent := filepath.Dir(curPath)
		if curPath == parent { // reached top-level directory
			return "", errRepoRootNotFound
		}
		curPath = parent
	}
}
