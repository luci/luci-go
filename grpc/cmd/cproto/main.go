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
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

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
// `inputDir`, `outputDir` and `importPaths` must all be given as absolute paths
// and must exist. `inputDir` must be under some import path. `protoFiles` must
// be given relative to `inputDir`. Generated files will be placed in
// `outputDir` based on their go_package option (so `outputDir` should be a root
// of Go package tree).
func compile(ctx context.Context, inputDir, outputDir string, protoFiles, importPaths []string, descSetOut string) error {
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
	sort.Strings(params)
	if !*disableGRPC && !*useGRPCPlugin {
		// Note: this enables deprecated protoc-gen-go grpc plugin.
		params = append(params, "plugins=grpc")
	}
	args = append(args, fmt.Sprintf("--go_out=%s:%s", strings.Join(params, ","), outputDir))

	// protoc-gen-go-grpc plugin arguments.
	if !*disableGRPC && *useGRPCPlugin {
		args = append(args, fmt.Sprintf("--go-grpc_out=%s", outputDir))
	}

	// protoc searches import paths purely lexicographically. Since we pass
	// import paths as absolute, we must use absolute paths for all input protos
	// as well, otherwise protoc gets confused.
	for _, f := range protoFiles {
		args = append(args, path.Join(inputDir, f))
	}

	logging.Infof(ctx, "protoc %s", strings.Join(args, " "))
	protoc := exec.Command("protoc", args...)
	protoc.Stdout = os.Stdout
	protoc.Stderr = os.Stderr
	return protoc.Run()
}

func run(ctx context.Context, inputDir string) error {
	// Stage all requested Go modules under a single root.
	inputs, err := stageInputs(ctx, inputDir, goModules, protoImportPaths)
	if err != nil {
		return err
	}
	defer os.RemoveAll(inputs.tmp)

	// Find .proto files in the staged input directory.
	protoFiles, err := findProtoFiles(inputs.inputDir)
	if err != nil {
		return err
	}
	if len(protoFiles) == 0 {
		return errors.Reason(".proto files not found").Err()
	}

	// Discover the proto package path by locating `inputDir` among import paths.
	var protoPkg string
	for _, p := range inputs.paths {
		if strings.HasPrefix(inputs.inputDir, p) {
			protoPkg = filepath.ToSlash(inputs.inputDir[len(p)+1:])
			break
		}
	}
	if protoPkg == "" {
		return errors.Reason("the input directory %q is outside of any proto path %v", inputs.inputDir, inputs.paths).Err()
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

	logging.Debugf(ctx, "Inputs: %s", inputs.inputDir)
	logging.Debugf(ctx, "Output: %s", inputs.outputDir)
	logging.Debugf(ctx, "Package: %s", protoPkg)
	logging.Debugf(ctx, "Files: %v", protoFiles)
	for _, p := range inputs.paths {
		logging.Debugf(ctx, "Proto path: %s", p)
	}

	// Compile all .proto files.
	err = compile(ctx, inputs.inputDir, inputs.outputDir, protoFiles, inputs.paths, descPath)
	if err != nil {
		return err
	}

	// protoc-gen-go puts generated files based on go_package option, rooting them
	// in the inputs.outputDir. We can't generally guess the Go package name just
	// based on proto file names, but we can extract it from the generated
	// descriptor.
	//
	// Doc:
	// https://developers.google.com/protocol-buffers/docs/reference/go-generated
	descSet, rawDesc, err := loadDescriptorSet(descPath)
	if err != nil {
		return errors.Annotate(err, "failed to load the descriptor set with generated files").Err()
	}

	generatedDesc := make([]*descriptorpb.FileDescriptorProto, 0, len(protoFiles))
	goPackages := stringset.New(0)

	// Since we use --include_imports, there may be a lot of descriptors in the
	// set. Visit only ones we care about.
	for _, protoFile := range protoFiles {
		fileDesc := descSet[path.Join(protoPkg, protoFile)]
		if fileDesc == nil {
			return errors.Reason("descriptor for %q is unexpectedly absent", protoFile).Err()
		}
		generatedDesc = append(generatedDesc, fileDesc)

		// "go_package" option is required now.
		goPackage := fileDesc.Options.GetGoPackage()
		if goPackage == "" {
			return errors.Reason("file %q has no go_package option set, it is required", protoFile).Err()
		}
		// Convert e.g. "foo/bar;pkgname" => "foo/bar".
		if idx := strings.LastIndex(goPackage, ";"); idx != -1 {
			goPackage = goPackage[:idx]
		}
		goPackages.Add(goPackage)

		// A file that protoc must have generated for us.
		goFile := filepath.Join(
			inputs.outputDir,
			filepath.FromSlash(goPackage),
			strings.TrimSuffix(protoFile, ".proto")+".pb.go",
		)
		if _, err := os.Stat(goFile); err != nil {
			return errors.Reason("could not find *.pb.go file generated from %q, is go_package option correct?", protoFile).Err()
		}

		// Transform .go files by adding pRPC stubs after gPRC stubs. Code generated
		// by protoc-gen-go-grpc plugin doesn't need this, since it uses interfaces
		// in the generated code (that pRPC implements) instead of concrete gRPC
		// types.
		if !*disableGRPC && !*useGRPCPlugin {
			var t transformer
			if err := t.transformGoFile(goFile); err != nil {
				return errors.Annotate(err, "could not transform %q", goFile).Err()
			}
		}

		// _test.proto's should go into the test package.
		if strings.HasSuffix(protoFile, "_test.proto") {
			newName := strings.TrimSuffix(goFile, ".go") + "_test.go"
			if err := os.Rename(goFile, newName); err != nil {
				return err
			}
		}
	}

	if !*disableGRPC && *withDiscovery {
		// We support generating a discovery file only when all generated *.pb.go
		// ended up in the same Go package. Otherwise it's not clear what package to
		// put the pb.discovery.go into.
		if goPackages.Len() != 1 {
			return errors.Reason(
				"cannot generate pb.discovery.go: generated *.pb.go files are in multiple packages %v",
				goPackages.ToSortedSlice(),
			).Err()
		}
		goPkg := goPackages.ToSlice()[0]
		out := filepath.Join(
			inputs.outputDir,
			filepath.FromSlash(goPkg),
			"pb.discovery.go",
		)
		if err := genDiscoveryFile(out, goPkg, generatedDesc, rawDesc); err != nil {
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
		fmt.Fprintln(os.Stderr, err.Error())
		exitCode := 1
		if rc, ok := exitcode.Get(err); ok {
			exitCode = rc
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
	paths     []string // absolute paths to search for protos
	inputDir  string   // absolute path to the directory with protos to compile
	outputDir string   // a directory with Go package root to put *.pb.go under
	tmp       string   // should be deleted (recursively) when done
}

// stageInputs stages a directory with Go Modules symlinked in approriate places
// and prepares corresponding --proto_path paths.
//
// If `inputDir` or any of given `protoImportPaths` are under any of the
// directories being staged, changes their paths to be rooted in the staged
// directory root. Such paths still point to the exact same directories, just
// through symlinks in the staging area.
func stageInputs(ctx context.Context, inputDir string, mods, protoImportPaths []string) (inputs *stagedInputs, err error) {
	// Try to find the main module (if running in modules mode). We'll put
	// generated files there.
	var mainMod *moduleInfo
	if os.Getenv("GO111MODULE") != "off" {
		var err error
		if mainMod, err = getModuleInfo("main"); err != nil {
			return nil, errors.Annotate(err, "could not find the main module").Err()
		}
		logging.Debugf(ctx, "The main module is %q at %q", mainMod.Path, mainMod.Dir)
	} else {
		logging.Debugf(ctx, "Running in GOPATH mode")
	}

	// If running in Go Modules mode, always put the main module and luci-go
	// modules into the proto path.
	modules := stringset.NewFromSlice(mods...)
	if mainMod != nil {
		modules.Add(mainMod.Path)
		if luciInfo, err := getModuleInfo("go.chromium.org/luci"); err == nil {
			modules.Add(luciInfo.Path)
		} else {
			logging.Warningf(ctx, "LUCI protos will be unavailable: %s", err)
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

	inputDir, err = relocatePath(inputDir)
	if err != nil {
		return nil, errors.Annotate(err, "bad input directory").Err()
	}

	// Prep import paths: union of GOPATH (if any), staged modules and
	// relocated `protoImportPaths`. Explicitly requested import paths come first.
	var paths []string
	for _, p := range protoImportPaths {
		p, err := relocatePath(p)
		if err != nil {
			return nil, errors.Annotate(err, "bad proto import path").Err()
		}
		paths = append(paths, p)
	}
	paths = append(paths, stagedRoot)
	paths = append(paths, build.Default.SrcDirs()...)

	// Include well-known *.proto files vendored into the luci-go repo.
	for _, p := range paths {
		abs := filepath.Join(p, "go.chromium.org", "luci", "grpc", "proto")
		if _, err := os.Stat(abs); err == nil {
			paths = append(paths, abs)
			break
		}
	}

	// In Go Modules mode, put outputs into staging directory. They'll end up in
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
			return nil, errors.Annotate(err, "the input directory %q is not under GOPATH %v", inputDir, srcDirs).Err()
		}
	}

	return &stagedInputs{
		paths:     paths,
		inputDir:  inputDir,
		outputDir: outputDir,
		tmp:       stagedRoot,
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
		return nil, errors.Annotate(err, "failed to resolve path of module %q", mod).Err()
	}
	return info, nil
}

// loadDescriptorSet reads and parses FileDescriptorSet proto.
//
// Returns it as a map: *.proto path in the registry => FileDescriptorProto,
// as well as raw byte blob.
func loadDescriptorSet(path string) (map[string]*descriptorpb.FileDescriptorProto, []byte, error) {
	blob, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	set := &descriptorpb.FileDescriptorSet{}
	if proto.Unmarshal(blob, set); err != nil {
		return nil, nil, err
	}
	mapping := make(map[string]*descriptorpb.FileDescriptorProto, len(set.File))
	for _, f := range set.File {
		mapping[f.GetName()] = f
	}
	return mapping, blob, nil
}
