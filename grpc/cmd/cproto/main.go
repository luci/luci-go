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
	"context"
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/exitcode"
)

var (
	verbose          = flag.Bool("verbose", false, "print debug messages to stderr")
	protoImportPaths = stringlistflag.Flag{}
	goModules        = stringlistflag.Flag{}
	pathMap          = stringmapflag.Value{}
	withDiscovery    = flag.Bool(
		"discovery", true,
		"generate pb.discovery.go file")
	descFile = flag.String(
		"desc",
		"",
		"write FileDescriptorSet file containing all the the .proto files and their transitive dependencies",
	)
	disableGRPC = flag.Bool(
		"disable-grpc", false,
		"disable grpc and prpc stubs generation, implies -discovery=false",
	)
	useGRPCPlugin = flag.Bool(
		"use-grpc-plugin", false,
		"use protoc-gen-go-grpc to generate gRPC stubs instead of protoc-gen-go",
	)
)

// Well-known Google proto packages -> go packages they are implemented in.
var googlePackages = map[string]string{
	"google/type/color.proto":          "google.golang.org/genproto/googleapis/type/color",
	"google/type/date.proto":           "google.golang.org/genproto/googleapis/type/date",
	"google/type/dayofweek.proto":      "google.golang.org/genproto/googleapis/type/dayofweek",
	"google/type/latlng.proto":         "google.golang.org/genproto/googleapis/type/latlng",
	"google/type/money.proto":          "google.golang.org/genproto/googleapis/type/money",
	"google/type/postal_address.proto": "google.golang.org/genproto/googleapis/type/postaladdress",
	"google/type/timeofday.proto":      "google.golang.org/genproto/googleapis/type/timeofday",

	"google/protobuf/any.proto":        "google.golang.org/protobuf/types/known/anypb",
	"google/protobuf/descriptor.proto": "google.golang.org/protobuf/types/descriptorpb",
	"google/protobuf/duration.proto":   "google.golang.org/protobuf/types/known/durationpb",
	"google/protobuf/empty.proto":      "google.golang.org/protobuf/types/known/emptypb",
	"google/protobuf/struct.proto":     "google.golang.org/protobuf/types/known/structpb",
	"google/protobuf/timestamp.proto":  "google.golang.org/protobuf/types/known/timestamppb",
	"google/protobuf/wrappers.proto":   "google.golang.org/protobuf/types/known/wrapperspb",

	"google/rpc/code.proto":          "google.golang.org/genproto/googleapis/rpc/code",
	"google/rpc/error_details.proto": "google.golang.org/genproto/googleapis/rpc/errdetails",
	"google/rpc/status.proto":        "google.golang.org/genproto/googleapis/rpc/status",

	"google/api/annotations.proto":    "google.golang.org/genproto/googleapis/api/annotations",
	"google/api/http.proto":           "google.golang.org/genproto/googleapis/api/annotations",
	"google/api/field_behavior.proto": "google.golang.org/genproto/googleapis/api/annotations",
	"google/api/resource.proto":       "google.golang.org/genproto/googleapis/api/annotations",
}

// compile runs protoc.
//
// `workDir` and `importPaths` must all be given as absolute paths and must
// exist. `workDir` must be under some import path. `protoFiles` must be given
// relative to `workDir`. Generated files will be placed in `workDir` side by
// side with proto files.
//
// Returns the proto package name matching the given `workDir`.
func compile(ctx context.Context, workDir string, protoFiles, importPaths []string, descSetOut string) (protoPkg string, err error) {
	// Discover the proto package path by locating `workDir` among import paths.
	// Remember this specific import path. We'll place generated Go files there,
	// so they end up in `workDir` in the end: protoc appends proto package path
	// to the output directory before writing files.
	var primaryDir string
	for _, p := range importPaths {
		if strings.HasPrefix(workDir, p) {
			primaryDir = p
			protoPkg = filepath.ToSlash(workDir[len(p)+1:])
			break
		}
	}
	if protoPkg == "" {
		return "", fmt.Errorf("the working directory %q is outside of any proto path %v", workDir, importPaths)
	}

	// Common protoc arguments.
	args := []string{
		"--descriptor_set_out=" + descSetOut,
		"--include_imports",
		"--include_source_info",
	}
	for _, p := range importPaths {
		args = append(args, "--proto_path="+p)
	}

	// protoc-gen-go plugin arguments.
	var params []string
	for k, v := range pathMap {
		params = append(params, fmt.Sprintf("M%s=%s", k, v))
	}
	if !*disableGRPC && !*useGRPCPlugin {
		// Note: this enables deprecated protoc-gen-go grpc plugin.
		params = append(params, "plugins=grpc")
	}
	args = append(args, fmt.Sprintf("--go_out=%s:%s", strings.Join(params, ","), primaryDir))

	// protoc-gen-go-grpc plugin arguments.
	if !*disableGRPC && *useGRPCPlugin {
		args = append(args, fmt.Sprintf("--go-grpc_out=%s", primaryDir))
	}

	// protoc searches import paths purely lexicographically. Since we pass
	// import paths as absolute, we must use absolute paths for all input protos
	// as well, otherwise protoc gets confused.
	for _, f := range protoFiles {
		args = append(args, path.Join(workDir, f))
	}

	logging.Infof(ctx, "protoc %s", strings.Join(args, " "))
	protoc := exec.Command("protoc", args...)
	protoc.Stdout = os.Stdout
	protoc.Stderr = os.Stderr
	return protoPkg, protoc.Run()
}

func run(ctx context.Context, workDir string) error {
	// Stage all requested Go modules under a single root.
	inputs, err := stageInputs(ctx, workDir, goModules, protoImportPaths)
	if err != nil {
		return err
	}
	defer os.RemoveAll(inputs.tmp)

	// Find .proto files in the staged working directory.
	protoFiles, err := findProtoFiles(inputs.workDir)
	if err != nil {
		return err
	}
	if len(protoFiles) == 0 {
		return fmt.Errorf(".proto files not found")
	}

	// Prep a path to the generated descriptors file.
	descPath := *descFile
	if descPath == "" {
		tmpDir, err := ioutil.TempDir("", "")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		descPath = filepath.Join(tmpDir, "package.desc")
	}

	// Compile all .proto files.
	protoPkg, err := compile(ctx, inputs.workDir, protoFiles, inputs.paths, descPath)
	if err != nil {
		return err
	}

	if *disableGRPC {
		return nil
	}

	// Switch back to using the original workDir instead of symlinked
	// inputs.workDir to avoid confusing "go list". We generated *.pb.go files
	// already, now need to process them.

	for _, p := range protoFiles {
		goFile := filepath.Join(workDir, strings.TrimSuffix(p, ".proto")+".pb.go")

		// Transform .go files by adding pRPC stubs after gPRC stubs. Code generated
		// by protoc-gen-go-grpc plugin doesn't need this, since it uses interfaces
		// in the generated code (that pRPC implements) instead of concrete gRPC
		// types.
		if !*useGRPCPlugin {
			var t transformer
			if err := t.transformGoFile(goFile); err != nil {
				return fmt.Errorf("could not transform %s: %s", goFile, err)
			}
		}

		if strings.HasSuffix(p, "_test.proto") {
			newName := strings.TrimSuffix(goFile, ".go") + "_test.go"
			if err := os.Rename(goFile, newName); err != nil {
				return err
			}
		}
	}

	if *withDiscovery {
		// Paths of processed *.proto files as they are registered in the proto
		// registry.
		registryPaths := make([]string, len(protoFiles))
		for i, p := range protoFiles {
			registryPaths[i] = path.Join(protoPkg, p)
		}
		if err := genDiscoveryFile(filepath.Join(workDir, "pb.discovery.go"), descPath, registryPaths); err != nil {
			return err
		}
	}

	return nil
}

func setupLogging(ctx context.Context) context.Context {
	lvl := logging.Warning
	if *verbose {
		lvl = logging.Debug
	}
	return logging.SetLevel(gologger.StdConfig.Use(context.Background()), lvl)
}

func usage() {
	fmt.Fprintln(os.Stderr,
		`Compiles all .proto files in a directory to .go with grpc+prpc support.
usage: cproto [flags] [dir]

Flags:`)
	flag.PrintDefaults()
}

func main() {
	flag.Var(
		&protoImportPaths,
		"proto-path",
		"additional proto import paths; "+
			"May be relative to CWD; "+
			"May be specified multiple times.")
	flag.Var(
		&goModules,
		"go-module",
		"make protos in the given module available in proto import path. "+
			"May be specified multiple times.")
	flag.Var(
		&pathMap,
		"map-package",
		"maps a proto path to a go package name. "+
			"May be specified multiple times.")
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(1)
	}
	dir := "."
	if flag.NArg() == 1 {
		dir = flag.Arg(0)
	}

	// Map well-known google protos to Go packages with their implementation.
	for k, v := range googlePackages {
		if _, ok := pathMap[k]; !ok {
			pathMap[k] = v
		}
	}

	ctx := setupLogging(context.Background())
	if err := run(ctx, dir); err != nil {
		exitCode := 1
		if rc, ok := exitcode.Get(err); ok {
			exitCode = rc
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(exitCode)
	}
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

type stagedInputs struct {
	paths   []string // absolute paths to search for protos
	workDir string   // absolute path to the working directory
	tmp     string   // should be deleted (recursively) when done
}

// stageInputs stages a directory with Go Modules symlinked in approriate places
// and prepares corresponding --proto_path paths.
//
// If `workDir` or any of given `protoImportPaths` are under any of the
// directories being staged, changes their paths to be rooted in the staged
// directory root. Such paths still point to the exact same directories, just
// through symlinks in the staging area.
func stageInputs(ctx context.Context, workDir string, mods, protoImportPaths []string) (inputs *stagedInputs, err error) {
	// If running in Go Modules mode, always put the main module and luci-go
	// modules into the proto path. Log but ignore errors (they may happen when
	// cproto is used outside of any module). If it is a serious issue, some later
	// step will fail.
	modules := stringset.NewFromSlice(mods...)
	if os.Getenv("GO111MODULE") != "off" {
		if mainInfo, err := getModuleInfo("main"); err == nil {
			modules.Add(mainInfo.Path)
		} else {
			logging.Warningf(ctx, "Could not find the main module: %s", err)
		}
		if luciInfo, err := getModuleInfo("go.chromium.org/luci"); err == nil {
			modules.Add(luciInfo.Path)
		} else {
			logging.Warningf(ctx, "LUCI protos may not be available: %s", err)
		}
	}

	// The directory with staged modules to use as --proto_path.
	stagedRoot, err := ioutil.TempDir("", "cproto")
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
			return "", errors.Reason("%q is not a directory", p).Err()
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

	workDir, err = relocatePath(workDir)
	if err != nil {
		return nil, errors.Annotate(err, "bad working directory").Err()
	}

	// Prep import paths: union of GOPATH (if any), staged modules and
	// relocated `protoImportPaths`.
	paths := append([]string{stagedRoot}, build.Default.SrcDirs()...)
	for _, p := range protoImportPaths {
		p, err := relocatePath(p)
		if err != nil {
			return nil, errors.Annotate(err, "bad proto import path").Err()
		}
		paths = append(paths, p)
	}

	// Include well-known *.proto files vendored into the luci-go repo.
	for _, p := range paths {
		abs := filepath.Join(p, "go.chromium.org", "luci", "grpc", "proto")
		if _, err := os.Stat(abs); err == nil {
			paths = append(paths, abs)
			break
		}
	}

	return &stagedInputs{
		paths:   paths,
		workDir: workDir,
		tmp:     stagedRoot,
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
func getModuleInfo(mod string) (info moduleInfo, err error) {
	args := []string{"-mod=readonly", "-m"}
	if mod != "main" {
		args = append(args, mod)
	}
	if err = goList(args, &info); err != nil {
		return info, errors.Annotate(err, "failed to resolve path of module %q", mod).Err()
	}
	return info, nil
}
