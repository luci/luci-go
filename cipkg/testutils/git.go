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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/index"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func createRepo(t testing.TB, dir string, files map[string]string) (*git.Repository, plumbing.Hash) {
	t.Helper()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init git repo: %s", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %s", err)
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory for %s: %s", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file %s: %s", name, err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("failed to add file %s: %s", name, err)
		}
	}

	commit, err := wt.Commit("commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %s", err)
	}

	return repo, commit
}

// InitGitRepo initializes a local git repository for testing.
func InitGitRepo(t testing.TB) (string, string) {
	t.Helper()

	dir := t.TempDir()
	_, commit := createRepo(t, dir, map[string]string{
		"file": "data",
	})

	return "file://" + filepath.ToSlash(dir), commit.String()
}

// InitGitRepoWithSubmodule initializes a local git repository with a submodule for testing.
func InitGitRepoWithSubmodule(t testing.TB) (string, string) {
	t.Helper()

	subDir := t.TempDir()
	mainDir := t.TempDir()

	// Create submodule repo
	_, subCommit := createRepo(t, subDir, map[string]string{
		"subfile": "subdata",
	})

	// Create main repo
	repo, _ := createRepo(t, mainDir, map[string]string{
		"file": "data",
	})
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %s", err)
	}

	// Add .gitmodules
	subUrl := "file://" + filepath.ToSlash(subDir)
	if err := os.WriteFile(filepath.Join(mainDir, ".gitmodules"), []byte(fmt.Sprintf("[submodule \"sub\"]\n\tpath = sub\n\turl = %s\n", subUrl)), 0644); err != nil {
		t.Fatalf("failed to create .gitmodules: %s", err)
	}
	if _, err := wt.Add(".gitmodules"); err != nil {
		t.Fatalf("failed to add .gitmodules: %s", err)
	}

	// Add gitlink to index
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatalf("failed to get index: %s", err)
	}
	idx.Entries = append(idx.Entries, &index.Entry{
		Name: "sub",
		Hash: subCommit,
		Mode: filemode.Submodule,
	})
	sort.Slice(idx.Entries, func(i, j int) bool {
		return idx.Entries[i].Name < idx.Entries[j].Name
	})
	if err := repo.Storer.SetIndex(idx); err != nil {
		t.Fatalf("failed to set index: %s", err)
	}

	// Commit submodule addition
	commit, err := wt.Commit("add sub", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %s", err)
	}

	return "file://" + filepath.ToSlash(mainDir), commit.String()
}
