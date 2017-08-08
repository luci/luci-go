// Copyright 2016 The LUCI Authors.
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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"
)

// tools keeps track of command-line tools that are available.
type tools struct {
	sync.Mutex

	pathMap map[string]string
}

func (t *tools) getLookup(command string) (string, error) {
	t.Lock()
	defer t.Unlock()

	path, ok := t.pathMap[command]
	if !ok {
		// This is the first lookup for the tool.
		var err error
		path, err = exec.LookPath(command)
		if err == nil {
			path, err = filepath.Abs(path)
		}
		if err != nil {
			path = ""
		}

		if t.pathMap == nil {
			t.pathMap = make(map[string]string)
		}
		t.pathMap[command] = path
	}

	if path == "" {
		// Lookup was attempted, but tool could not be found.
		return "", errors.Reason("tool %q is not available", command).Err()
	}
	return path, nil
}

func (t *tools) genericTool(name string) (*genericTool, error) {
	exe, err := t.getLookup(name)
	if err != nil {
		return nil, err
	}
	return &genericTool{
		exe: exe,
	}, nil
}

func (t *tools) python() (*genericTool, error) { return t.genericTool("python") }
func (t *tools) docker() (*genericTool, error) { return t.genericTool("docker") }

func (t *tools) git() (*gitTool, error) {
	exe, err := t.getLookup("git")
	if err != nil {
		return nil, err
	}
	return &gitTool{
		exe: exe,
	}, nil
}

func (t *tools) goTool(goPath []string) (*goTool, error) {
	exe, err := t.getLookup("go")
	if err != nil {
		return nil, err
	}
	return &goTool{
		exe:    exe,
		goPath: goPath,
	}, nil
}

func (t *tools) kubectl(context string) (*kubeTool, error) {
	exe, err := t.getLookup("kubectl")
	if err != nil {
		return nil, err
	}
	return &kubeTool{
		exe: exe,
		ctx: context,
	}, nil
}

func (t *tools) gcloud(project string) (*gcloudTool, error) {
	exe, err := t.getLookup("gcloud")
	if err != nil {
		return nil, err
	}
	return &gcloudTool{
		exe:     exe,
		project: project,
	}, nil
}

func (t *tools) aedeploy(goPath []string) (*aedeployTool, error) {
	exe, err := t.getLookup("aedeploy")
	if err != nil {
		return nil, err
	}
	return &aedeployTool{
		exe:    exe,
		goPath: goPath,
	}, nil
}

type genericTool struct {
	exe string
}

func (t *genericTool) exec(command string, args ...string) *workExecutor {
	return execute(t.exe, append([]string{command}, args...)...)
}

type gitTool struct {
	exe string
}

func (t *gitTool) exec(gitDir string, command string, args ...string) *workExecutor {
	return execute(t.exe, append([]string{"-C", gitDir, command}, args...)...)
}

func (t *gitTool) clone(c context.Context, src, dst string) error {
	return t.exec(".", "clone", src, dst).check(c)
}

func (t *gitTool) getHEAD(c context.Context, gitDir string) (string, error) {
	x := t.exec(gitDir, "rev-parse", "HEAD")
	if err := x.check(c); err != nil {
		return "", err
	}
	rev := strings.TrimSpace(x.stdout.String())
	if len(rev) == 0 {
		return "", errors.New("invalid empty revision")
	}
	return rev, nil
}

func (t *gitTool) getMergeBase(c context.Context, gitDir, remote string) (string, error) {
	x := t.exec(gitDir, "merge-base", "HEAD", remote)
	if err := x.check(c); err != nil {
		return "", err
	}
	rev := strings.TrimSpace(x.stdout.String())
	if len(rev) == 0 {
		return "", errors.New("invalid empty revision")
	}
	return rev, nil
}

func (t *gitTool) getRevListCount(c context.Context, gitDir string) (int, error) {
	x := t.exec(gitDir, "rev-list", "--count", "HEAD")
	if err := x.check(c); err != nil {
		return 0, err
	}

	output := strings.TrimSpace(x.stdout.String())
	v, err := strconv.Atoi(output)
	if err != nil {
		return 0, errors.Annotate(err, "failed to parse rev-list count").
			InternalReason("output(%s)", output).Err()
	}
	return v, nil
}

type goTool struct {
	exe    string
	goPath []string
}

func (t *goTool) exec(subCommand string, args ...string) *workExecutor {
	return execute(t.exe, append([]string{subCommand}, args...)...).loadEnv(os.Environ()).envPath("GOPATH", t.goPath...)
}

func (t *goTool) build(c context.Context, out string, pkg ...string) error {
	wtd := withTempDir
	if out != "" {
		wtd = func(f func(string) error) error {
			return f(out)
		}
	}

	return wtd(func(tdir string) error {
		return t.exec("build", pkg...).cwd(tdir).check(c)
	})
}

type aedeployTool struct {
	exe    string
	goPath []string
}

func (t *aedeployTool) bootstrap(x *workExecutor) *workExecutor {
	return x.bootstrap(t.exe).loadEnv(os.Environ()).envPath("GOPATH", t.goPath...)
}

type gcloudTool struct {
	exe     string
	project string
}

func (t *gcloudTool) exec(command string, args ...string) *workExecutor {
	return execute(t.exe, append([]string{"--project", t.project, command}, args...)...)
}
