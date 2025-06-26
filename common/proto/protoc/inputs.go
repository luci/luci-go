// Copyright 2021 The LUCI Authors.
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

package protoc

import (
	"context"
	"encoding/json"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// StagedInputs represents a staging directory with Go modules symlinked into
// a way that `protoc` sees a consistent --proto_path.
type StagedInputs struct {
	Paths        []string // absolute paths to search for protos
	InputDir     string   // absolute path to the directory with protos to compile
	OutputDir    string   // a directory with Go package root to put *.pb.go under
	ProtoFiles   []string // names of proto files in InputDir
	ProtoPackage string   // proto package path matching InputDir
	tmp          string   // if not empty, should be deleted (recursively) when done
}

// Cleanup removes the temporary staging directory.
func (s *StagedInputs) Cleanup() error {
	if s.tmp != "" {
		return os.RemoveAll(s.tmp)
	}
	return nil
}

// StageGoInputs stages a directory with Go Modules symlinked in appropriate
// places and prepares corresponding --proto_path paths.
//
// If `inputDir` or any of given `protoImportPaths` are under any of the
// directories being staged, changes their paths to be rooted in the staged
// directory root. Such paths still point to the exact same directories, just
// through symlinks in the staging area.
func StageGoInputs(ctx context.Context, inputDir string, mods, rootMods, protoImportPaths []string) (inputs *StagedInputs, err error) {
	// Try to find the main module (if running in modules mode). We'll put
	// generated files there.
	var mainMod *moduleInfo
	if os.Getenv("GO111MODULE") != "off" {
		var err error
		if mainMod, err = getModuleInfo("main"); err != nil {
			return nil, errors.Fmt("could not find the main module: %w", err)
		}
		logging.Debugf(ctx, "The main module is %q at %q", mainMod.Path, mainMod.Dir)
	} else {
		logging.Debugf(ctx, "Running in GOPATH mode")
	}

	// If running in Go Modules mode, always put the main module and luci-go
	// modules into the proto path.
	modules := stringset.NewFromSlice(mods...)
	modules.AddAll(rootMods)
	if mainMod != nil {
		modules.Add(mainMod.Path)
		if luciInfo, err := getModuleInfo("go.chromium.org/luci"); err == nil {
			modules.Add(luciInfo.Path)
		} else {
			logging.Warningf(ctx, "LUCI protos will be unavailable: %s", err)
		}
	}

	// The directory with staged modules to use as --proto_path.
	stagedRoot, err := os.MkdirTemp("", "cproto")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(stagedRoot)
		}
	}()

	// Stage requested modules into a temp directory using symlinks.
	mapping, err := stageModules(stagedRoot, modules.ToSortedSlice())
	if err != nil {
		return nil, err
	}

	// "Relocate" paths into the staging directory. It doesn't change what they
	// point to, just makes them resolve through symlinks in the staging
	// directory, if possible. Also convert path to absolute and verify they are
	// existing directories. This is needed to make relative paths calculation in
	// protoc happy.
	relocatePath := func(p string) (string, error) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		switch s, err := os.Stat(abs); {
		case err != nil:
			return "", err
		case !s.IsDir():
			return "", errors.Fmt("%q is not a directory", p)
		}
		for pre, post := range mapping {
			if abs == pre {
				return post, nil
			}
			if strings.HasPrefix(abs, pre+string(filepath.Separator)) {
				return post + abs[len(pre):], nil
			}
		}
		return abs, nil
	}

	inputDir, err = relocatePath(inputDir)
	if err != nil {
		return nil, errors.Fmt("bad input directory: %w", err)
	}

	// Prep import paths: union of GOPATH (if any), staged modules and
	// relocated `protoImportPaths`.
	var paths []string
	paths = append(paths, stagedRoot)
	for _, mod := range rootMods {
		paths = append(paths, filepath.Join(stagedRoot, mod))
	}
	paths = append(paths, build.Default.SrcDirs()...)

	// Explicitly requested import paths come last. This is needed to make sure
	// protoc is not confused if an input *.proto name shows up in one of the
	// protoImportPaths. It already shows up in the stagedRoot proto path (by
	// construction). If protoc sees the input proto in another --proto_path
	// that comes *before* stagedRoot, it spits out this confusing error:
	//
	//   Input is shadowed in the --proto_path by "<explicitly requested path>".
	//   Either use the latter file as your input or reorder the --proto_path so
	//   that the former file's location comes first.
	//
	// By putting protoImportPaths last, we "reorder the --proto_path so that the
	// former file's location comes first".
	for _, p := range protoImportPaths {
		p, err := relocatePath(p)
		if err != nil {
			return nil, errors.Fmt("bad proto import path: %w", err)
		}
		paths = append(paths, p)
	}

	// Include googleapis proto files vendored into the luci-go repo.
	for _, p := range paths {
		abs := filepath.Join(p, "go.chromium.org", "luci", "common", "proto", "googleapis")
		if _, err := os.Stat(abs); err == nil {
			paths = append(paths, abs)
			break
		}
	}

	// In Go Modules mode, put outputs into a staging directory. They'll end up in
	// the correct module based on go_package definitions. In GOPATH mode, put
	// them into the GOPATH entry that contains the input directory.
	var outputDir string
	if mainMod != nil {
		outputDir = stagedRoot
	} else {
		srcDirs := build.Default.SrcDirs()
		for _, p := range srcDirs {
			if strings.HasPrefix(inputDir, p) {
				outputDir = p
				break
			}
		}
		if outputDir == "" {
			return nil, errors.Fmt("the input directory %q is not under GOPATH %v", inputDir, srcDirs)
		}
	}

	// Find .proto files in the staged input directory.
	protoFiles, err := findProtoFiles(inputDir)
	if err != nil {
		return nil, err
	}
	if len(protoFiles) == 0 {
		return nil, errors.Fmt("%s: no .proto files found", inputDir)
	}

	// Discover the proto package path by locating `inputDir` among import paths.
	var protoPkg string
	for _, p := range paths {
		if strings.HasPrefix(inputDir, p) {
			protoPkg = filepath.ToSlash(inputDir[len(p)+1:])
			break
		}
	}
	if protoPkg == "" {
		return nil, errors.Fmt("the input directory %q is outside of any proto path %v", inputDir, paths)
	}

	return &StagedInputs{
		Paths:        paths,
		InputDir:     inputDir,
		OutputDir:    outputDir,
		ProtoFiles:   protoFiles,
		ProtoPackage: protoPkg,
		tmp:          stagedRoot,
	}, nil
}

// StageGenericInputs just prepares StagedInputs from some generic directories
// on disk.
//
// Unlike StageGoInputs it doesn't try to derive any information from what's
// there. Just converts paths to absolute and discovers *.proto files.
func StageGenericInputs(ctx context.Context, inputDir string, protoImportPaths []string) (*StagedInputs, error) {
	absInputDir, err := filepath.Abs(inputDir)
	if err != nil {
		return nil, errors.Fmt("could not make path %q absolute: %w", inputDir, err)
	}

	absImportPaths := make([]string, 0, len(protoImportPaths)+1)
	includesInputDir := false
	protoPackageDir := ""
	for _, path := range protoImportPaths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, errors.Fmt("could not make path %q absolute: %w", path, err)
		}
		absImportPaths = append(absImportPaths, abs)
		if strings.HasPrefix(absInputDir, abs+string(filepath.Separator)) && !includesInputDir {
			includesInputDir = true
			protoPackageDir = filepath.ToSlash(absInputDir[len(abs)+1:])
		}
	}

	// Add the input directory to the proto import path only if it is not already
	// included via some existing import path. Adding it twice confuses protoc.
	// Not adding it at all breaks relative imports.
	if !includesInputDir {
		absImportPaths = append(absImportPaths, absInputDir)
		protoPackageDir = "."
	}

	protoFiles, err := findProtoFiles(absInputDir)
	if err != nil {
		return nil, err
	}
	if len(protoFiles) == 0 {
		return nil, errors.New(".proto files not found")
	}

	return &StagedInputs{
		Paths:        absImportPaths,
		InputDir:     absInputDir,
		OutputDir:    absInputDir, // drop generated files (if any) right there
		ProtoFiles:   protoFiles,
		ProtoPackage: protoPackageDir,
	}, nil
}

// stageModules symlinks given Go modules (and only them) at their module paths.
//
// Returns a map "original abs path => symlinked abs path".
func stageModules(root string, mods []string) (map[string]string, error) {
	mapping := make(map[string]string, len(mods))
	for _, m := range mods {
		info, err := getModuleInfo(m)
		if err != nil {
			return nil, err
		}
		dest := filepath.Join(root, filepath.FromSlash(m))
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return nil, err
		}
		if err := os.Symlink(info.Dir, dest); err != nil {
			return nil, err
		}
		mapping[info.Dir] = dest
	}
	return mapping, nil
}

type moduleInfo struct {
	Path string // e.g. "go.chromium.org/luci"
	Dir  string // e.g. "/home/work/gomodcache/.../luci"
}

// getModuleInfo returns the information about a module.
//
// Pass "main" to get the information about the main module.
func getModuleInfo(mod string) (*moduleInfo, error) {
	args := []string{"-mod=readonly", "-m"}
	if mod != "main" {
		args = append(args, mod)
	}
	info := &moduleInfo{}
	if err := goList(args, info); err != nil {
		return nil, errors.Fmt("failed to resolve path of module %q: %w", mod, err)
	}
	return info, nil
}

// goList calls "go list -json <args>" and parses the result.
func goList(args []string, out any) error {
	cmd := exec.Command("go", append([]string{"list", "-json"}, args...)...)
	buf, err := cmd.Output()
	if err != nil {
		if er, ok := err.(*exec.ExitError); ok && len(er.Stderr) > 0 {
			return errors.Fmt("%s", er.Stderr)
		}
		return err
	}
	return json.Unmarshal(buf, out)
}

// findProtoFiles returns .proto files in dir. The returned file paths
// are relative to dir.
func findProtoFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.proto"))
	if err != nil {
		return nil, err
	}
	for i, f := range files {
		files[i] = filepath.Base(f)
	}
	return files, err
}
