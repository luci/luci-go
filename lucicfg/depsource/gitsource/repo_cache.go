// Copyright 2025 The LUCI Authors.
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

package gitsource

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"

	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/logging"
)

type RepoCache struct {
	// path/to/<Cache.repoRoot>/<sha256(remoteUrl)>
	repoRoot string

	// batchProc will reject missing blobs
	batchProc batchProc
}

// Shutdown terminates long-running processes which may be associated with this
// RepoCache.
//
// It is safe to use RepoCache after calling Shutdown (but these long-running
// processes may be brought back up again)
func (r *RepoCache) Shutdown() {
	r.batchProc.shutdown()
}

func newRepoCache(repoRoot string) (*RepoCache, error) {
	ret := &RepoCache{repoRoot: repoRoot}

	if err := os.MkdirAll(ret.repoRoot, 0777); err != nil {
		return nil, fmt.Errorf("making root dir: %w", err)
	}

	ret.batchProc.mkCmd = ret.mkGitCmd
	return ret, nil
}

func (r *RepoCache) mkGitCmd(ctx context.Context, args []string) *exec.Cmd {
	fullArgs := append([]string{"--git-dir"}, r.repoRoot)
	fullArgs = append(fullArgs, args...)
	ret := exec.CommandContext(ctx, "git", fullArgs...)
	logging.Debugf(ctx, "running: %s", ret)
	if logging.GetLevel(ctx) <= logging.Debug {
		ret.Stdout = os.Stdout
		ret.Stderr = os.Stderr
	}
	return ret
}

func (r *RepoCache) fixCmdErr(cmd *exec.Cmd, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("running %s: %w", cmd, err)
}

// git runs the command, returning an error unless the command succeeded.
func (r *RepoCache) git(ctx context.Context, args ...string) error {
	cmd := r.mkGitCmd(ctx, args)
	return r.fixCmdErr(cmd, cmd.Run())
}

// gitOutput returns the stdout from this git command
func (r *RepoCache) gitOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := r.mkGitCmd(ctx, args)
	cmd.Stdout = nil
	out, err := cmd.Output()
	return out, r.fixCmdErr(cmd, err)
}

// gitCombinedOutput returns the stdout from this git command
func (r *RepoCache) gitCombinedOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := r.mkGitCmd(ctx, args)
	cmd.Stdout = nil
	cmd.Stderr = nil
	out, err := cmd.CombinedOutput()
	return out, r.fixCmdErr(cmd, err)
}

func (r *RepoCache) setConfigBlock(ctx context.Context, cb configBlock) error {
	versionKey := fmt.Sprintf("%s.gitsourceVersion", cb.section)
	versionStr := cb.hash()

	val, err := r.gitOutput(ctx, "config", versionKey)
	if err == nil {
		if string(val[:len(val)-1]) == versionStr {
			return nil // present and correct version
		}
		// need to reset
		if len(val) > 0 {
			if err := r.git(ctx, "config", "--remove-section", cb.section); err != nil {
				return err
			}
		}
	} else if err := filterCode(err, 1); err != nil {
		return err
	}
	// missing

	for key, value := range cb.config {
		if err := r.git(ctx, "config", fmt.Sprintf("%s.%s", cb.section, key), value); err != nil {
			return err
		}
	}
	return r.git(ctx, "config", versionKey, versionStr)
}

type configBlock struct {
	section string
	config  map[string]string
}

func (r configBlock) hash() string {
	h := sha1.New()
	_, err := fmt.Fprintf(h, "section %q\n", r.section)
	if err != nil {
		panic(err)
	}
	for _, key := range slices.Sorted(maps.Keys(r.config)) {
		_, err := fmt.Fprintf(h, "%q %q\n", key, r.config[key])
		if err != nil {
			panic(err)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func filterCode(err error, codes ...int) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if slices.Contains(codes, exitErr.ExitCode()) {
			return nil
		}
	}
	return err
}
