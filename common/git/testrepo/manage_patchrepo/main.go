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

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/git/testrepo"
)

var managePatchrepo = bytes.Join([][]byte{
	[]byte("#!/bin/bash -ex\n"),
	[]byte("cd `dirname -- $0`\n"),
	[]byte("go run go.chromium.org/luci/common/git/testrepo/manage_patchrepo \"$@\"\n"),
}, nil)

func parseArgs(withOverwrite bool, args []string) (patchDir, repoDir string, overwrite bool, err error) {
	dest, err := os.Getwd()
	if err != nil {
		return
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.StringVar(&patchDir, "patches", filepath.Join(dest, testrepo.PatchesDir), "")
	fs.StringVar(&repoDir, "repo", filepath.Join(dest, testrepo.TestgitRepoDir), "")
	if withOverwrite {
		fs.BoolVar(&overwrite, "overwrite", false, "")
	}

	err = fs.Parse(args)
	if err != nil {
		return
	}
	if fs.NArg() > 0 {
		err = fmt.Errorf("extra args: %q", fs.Args())
	}
	return
}

func doToRepo(args []string) error {
	patchDir, repoDir, overwrite, err := parseArgs(true, args)
	if err != nil {
		return err
	}
	baseDir := filepath.Dir(patchDir)
	path, err := testrepo.ToGit(context.Background(), baseDir, repoDir, overwrite)
	if err != nil {
		return err
	}
	log.Printf("created repo: %s\n", path)
	return nil
}

func doToPatches(args []string) error {
	patchDir, repoDir, _, err := parseArgs(false, args)
	if err != nil {
		return err
	}
	baseDir := filepath.Dir(patchDir)
	return testrepo.ToPatches(context.Background(), baseDir, repoDir)
}

func doInit(args []string) (err error) {
	var dest string
	switch len(args) {
	case 0:
		dest, err = os.Getwd()
		if err != nil {
			return
		}
	case 1:
		dest = args[0]
	default:
		return fmt.Errorf("expected 0 or 1 argument, got %d: %q", len(args), args)
	}

	if err := os.MkdirAll(dest, 0777); err != nil {
		return err
	}
	dir, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	if len(dir) > 0 {
		return fmt.Errorf("cannot init %q: directory exists and is non-empty", dest)
	}

	err = os.WriteFile(filepath.Join(dest, ".gitignore"), fmt.Appendf(nil, "/%s\n", testrepo.TestgitRepoDir), 0666)
	if err != nil {
		return
	}
	return os.WriteFile(filepath.Join(dest, "manage_patchrepo.sh"), managePatchrepo, 0777)
}

func main() {
	var subcmd string
	if len(os.Args) >= 2 {
		subcmd = os.Args[1]
	}

	var err error
	switch subcmd {
	case "to-repo":
		err = doToRepo(os.Args[2:])
	case "to-patches":
		err = doToPatches(os.Args[2:])
	case "init":
		err = doInit(os.Args[2:])
	default:
		err = fmt.Errorf("unknown subcommand %q", subcmd)
	}
	if err == nil {
		return
	}

	log.Print(err)
	log.Print()

	log.Print(strings.Join([]string{
		"usage:",
		"",
		"If /path/to/basedir is omitted, it uses $PWD",
		"If /path/to/patches is omitted, it uses $PWD/" + testrepo.PatchesDir,
		"If /path/to/repo is omitted, it uses $PWD/" + testrepo.TestgitRepoDir,
		"",
		"  manage_patchrepo init [/path/to/basedir]",
		"  manage_patchrepo to-repo [-patches /path/to/patches] [-repo /path/to/repo/dir] [-overwrite]",
		"  manage_patchrepo to-patches [-patches /path/to/patches] [-repo /path/to/repo/dir]",
		"",
		"Use `init` to populate a base directory (usually a Go testdata directory",
		"or subdirectory) with a helper script and .gitignore file.",
		"",
		"Use `to-repo` to generate a real repo on-disk with the contents of the patch",
		"directory.",
		"",
		"Use `to-patches` to generate text patches from the real repo on-disk.",
	}, "\n"))
	os.Exit(1)
}
