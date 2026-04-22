// Copyright 2026 The LUCI Authors.
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

//go:build !aix || !ppc64

package testutils

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
)

// InitGitRepo initializes a local git repository for testing.
func InitGitRepo(t testing.TB) (string, string) {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init git repo: %s", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %s", err)
	}

	f, err := os.Create(filepath.Join(dir, "file"))
	if err != nil {
		t.Fatalf("failed to create file: %s", err)
	}
	f.Close()
	_, err = wt.Add("file")
	if err != nil {
		t.Fatalf("failed to add file: %s", err)
	}

	commit, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %s", err)
	}

	return "test://" + filepath.ToSlash(dir), commit.String()
}

// testRepoLoader loads git repository from filesystem for test protocol.
type testRepoLoader struct{}

func (l *testRepoLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	path := ep.Path
	if ep.Host != "" {
		path = ep.Host + ":" + path
	}
	r, err := git.PlainOpen(filepath.FromSlash(path))
	if err != nil {
		return nil, err
	}
	return r.Storer, nil
}

func init() {
	client.InstallProtocol("test", server.NewClient(&testRepoLoader{}))
}
