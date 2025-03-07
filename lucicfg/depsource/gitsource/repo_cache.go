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
	"os"
	"sync"

	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/logging"
)

type RepoCache struct {
	// path/to/<Cache.repoRoot>/<sha256(remoteUrl)
	repoRoot string

	// map ref -> resolved
	lsRemoteCache map[string]string
	lsRemoteMu    sync.RWMutex

	// batchProc is a `git --no-lazy-fetch cat-file --batch` process. Interacting with this via
	// batchProcStdin/batchProcStdout needs to hold batchProcMu until the entire
	// response has been consumed from batchProcStdout.
	//batchProc     batchProc
	//batchProcLazy batchProc
}

func newRepoCache(repoRoot string) (*RepoCache, error) {
	ret := &RepoCache{repoRoot: repoRoot}

	if err := os.MkdirAll(ret.repoRoot, 0777); err != nil {
		return nil, fmt.Errorf("making root dir: %w", err)
	}

	//ret.batchProc.mkCmd = ret.mkGitCmd
	//ret.batchProcLazy.mkCmd = ret.mkGitCmd
	//ret.batchProcLazy.lazy = true
	return ret, nil
}

func (c *RepoCache) mkGitCmd(ctx context.Context, args []string) *exec.Cmd {
	fullArgs := append([]string{"--git-dir"}, c.repoRoot)
	fullArgs = append(fullArgs, args...)
	ret := exec.CommandContext(ctx, "git", fullArgs...)
	logging.Infof(ctx, "running: %s", ret)
	return ret
}

func (c *RepoCache) fixCmdErr(cmd *exec.Cmd, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("running %s: %w", cmd, err)
}

// git runs the command, returning an error unless the command succeeded.
func (c *RepoCache) git(ctx context.Context, args ...string) error {
	cmd := c.mkGitCmd(ctx, args)
	return c.fixCmdErr(cmd, cmd.Run())
}

// gitOutput returns the stdout from this git command
func (c *RepoCache) gitOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := c.mkGitCmd(ctx, args)
	out, err := cmd.Output()
	return out, c.fixCmdErr(cmd, err)
}

// gitTest returns:
//
//   - (true, nil) if the command succeeded
//   - (false, nil) if the command ran but returned a non-zero exit code.
//   - (false, err) if the command failed to run or ran abnormally.
func (c *RepoCache) gitTest(ctx context.Context, args ...string) (bool, error) {
	cmd := c.mkGitCmd(ctx, args)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		err = nil
	}
	return false, c.fixCmdErr(cmd, err)
}

func (c *RepoCache) setConfigBlock(ctx context.Context, cb configBlock) error {
	versionKey := fmt.Sprintf("%s.gitsourceVersion", cb.section)
	versionStr := cb.hash()

	val, err := c.gitOutput(ctx, "config", "get", versionKey)
	if err == nil {
		if string(val[:len(val)-1]) == versionStr {
			return nil // present and correct version
		}
		// need to reset
		if len(val) > 0 {
			if err := c.git(ctx, "config", "remove-section", cb.section); err != nil {
				return err
			}
		}
	} else if err := filterCode(err, 1); err != nil {
		return err
	} else {
		// missing
	}

	for key, value := range cb.config {
		if err := c.git(ctx, "config", "set", fmt.Sprintf("%s.%s", cb.section, key), value); err != nil {
			return err
		}
	}
	return c.git(ctx, "config", "set", versionKey, versionStr)
}

type configBlock struct {
	section string
	config  map[string]string
}

func (c configBlock) hash() string {
	h := sha1.New()
	fmt.Fprintln(h, "section", c.section)
	for key, val := range c.config {
		fmt.Fprintln(h, key, val)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func filterCode(err error, codes ...int) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		ec := exitErr.ExitCode()
		for _, code := range codes {
			if ec == code {
				return nil
			}
		}
	}
	return err
}
