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
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/exitcode"
)

var (
	verbose          = flag.Bool("verbose", false, "print debug messages to stderr")
	protoImportPaths = stringlistflag.Flag{}
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

// compile runs protoc on protoFiles. protoFiles must be relative to dir.
func compile(ctx context.Context, gopath, importPaths, protoFiles []string, dir, descSetOut string) (outDirFS, outDirProto string, err error) {
	// make it absolute to find in $GOPATH and because protoc wants paths
	// to be under proto paths.
	if dir, err = filepath.Abs(dir); err != nil {
		return "", "", err
	}

	// By default place go files in CWD,
	// unless proto files are under a $GOPATH/src.
	goOut := "."

	// Combine user-defined proto paths with $GOPATH/src.
	allProtoPaths := make([]string, 0, len(importPaths)+len(gopath)+1)
	for _, p := range importPaths {
		if p, err = filepath.Abs(p); err != nil {
			return "", "", err
		}
		allProtoPaths = append(allProtoPaths, p)
	}
	for _, p := range gopath {
		path := filepath.Join(p, "src")
		if info, err := os.Stat(path); os.IsNotExist(err) || !info.IsDir() {
			continue
		} else if err != nil {
			return "", "", err
		}
		allProtoPaths = append(allProtoPaths, path)

		// If the dir is under $GOPATH/src, generate .go files near .proto files.
		if strings.HasPrefix(dir, path) {
			goOut = path
		}

		// Include well-known protobuf types.
		wellKnownProtoDir := filepath.Join(path, "go.chromium.org", "luci", "grpc", "proto")
		if info, err := os.Stat(wellKnownProtoDir); err == nil && info.IsDir() {
			allProtoPaths = append(allProtoPaths, wellKnownProtoDir)
		}
	}

	// Find where Go files will be generated.
	for _, p := range allProtoPaths {
		if strings.HasPrefix(dir, p) {
			outDirFS = filepath.Join(goOut, dir[len(p):])
			outDirProto = dir[len(p)+1:]
			break
		}
	}
	if outDirFS == "" {
		return "", "", fmt.Errorf("proto files are neither under $GOPATH/src nor -proto-path")
	}

	args := []string{
		"--descriptor_set_out=" + descSetOut,
		"--include_imports",
		"--include_source_info",
	}
	for _, p := range allProtoPaths {
		args = append(args, "--proto_path="+p)
	}

	var params []string
	for k, v := range pathMap {
		params = append(params, fmt.Sprintf("M%s=%s", k, v))
	}
	if !*disableGRPC && !*useGRPCPlugin {
		// Note: this enables deprecated protoc-gen-go grpc plugin.
		params = append(params, "plugins=grpc")
	}
	args = append(args, fmt.Sprintf("--go_out=%s:%s", strings.Join(params, ","), goOut))

	if !*disableGRPC && *useGRPCPlugin {
		args = append(args, fmt.Sprintf("--go-grpc_out=%s", goOut))
	}

	for _, f := range protoFiles {
		// We must prepend an go-style absolute path to the filename otherwise
		// protoc will complain that the files we specify here are not found
		// in any of proto-paths.
		//
		// We cannot specify --proto-path=. because of the following scenario:
		// we have file structure
		// - A
		//   - x.proto, imports "y.proto"
		//   - y.proto
		// - B
		//   - z.proto, imports "github.com/user/repo/A/x.proto"
		// If cproto is executed in B, proto path does not include A, so y.proto
		// is not found.
		// The solution is to always use absolute paths.
		args = append(args, path.Join(dir, f))
	}
	logging.Infof(ctx, "protoc %s", strings.Join(args, " "))
	protoc := exec.Command("protoc", args...)
	protoc.Stdout = os.Stdout
	protoc.Stderr = os.Stderr
	return outDirFS, outDirProto, protoc.Run()
}

func run(ctx context.Context, goPath []string, dir string) error {
	if s, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("%s does not exist", dir)
	} else if err != nil {
		return err
	} else if !s.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	// Find .proto files
	protoFiles, err := findProtoFiles(dir)
	if err != nil {
		return err
	}
	if len(protoFiles) == 0 {
		return fmt.Errorf(".proto files not found")
	}

	// Compile all .proto files.
	descPath := *descFile
	if descPath == "" {
		tmpDir, err := os.MkdirTemp("", "")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		descPath = filepath.Join(tmpDir, "package.desc")
	}

	outDirFS, outDirProto, err := compile(ctx, goPath, protoImportPaths, protoFiles, dir, descPath)
	if err != nil {
		return err
	}

	if *disableGRPC {
		return nil
	}

	for _, p := range protoFiles {
		goFile := filepath.Join(outDirFS, strings.TrimSuffix(p, ".proto")+".pb.go")

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
			registryPaths[i] = path.Join(outDirProto, p)
		}
		if err := genDiscoveryFile(filepath.Join(outDirFS, "pb.discovery.go"), descPath, registryPaths); err != nil {
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

If the dir is not under $GOPATH/src, places generated Go files relative to $CWD.

Flags:`)
	flag.PrintDefaults()
}

func main() {
	flag.Var(
		&protoImportPaths,
		"proto-path",
		"additional proto import paths besides $GOPATH/src; "+
			"May be relative to CWD; "+
			"May be specified multiple times.")
	flag.Var(
		&pathMap,
		"map-package",
		"Maps a proto path to a go package name. "+
			"May be specified multiple times.")
	flag.Usage = usage
	flag.Parse()

	for k, v := range googlePackages {
		if _, ok := pathMap[k]; !ok {
			pathMap[k] = v
		}
	}

	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(1)
	}
	dir := "."
	if flag.NArg() == 1 {
		dir = flag.Arg(0)
	}

	c := setupLogging(context.Background())
	goPath := strings.Split(os.Getenv("GOPATH"), string(filepath.ListSeparator))
	if err := run(c, goPath, dir); err != nil {
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

// isInPackage returns true if the filename is a part of the package.
func isInPackage(fileName string, pkg string) (bool, error) {
	dir, err := filepath.Abs(filepath.Dir(fileName))
	if err != nil {
		return false, err
	}
	dir = path.Clean(dir)
	pkg = path.Clean(pkg)
	if !strings.HasSuffix(dir, pkg) {
		return false, nil
	}

	src := strings.TrimSuffix(dir, pkg)
	src = path.Clean(src)
	goPaths := strings.Split(os.Getenv("GOPATH"), string(filepath.ListSeparator))
	for _, goPath := range goPaths {
		if filepath.Join(goPath, "src") == src {
			return true, nil
		}
	}
	return false, nil
}
