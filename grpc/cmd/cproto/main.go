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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/exitcode"
)

var (
	verbose          = flag.Bool("verbose", false, "print debug messages to stderr")
	protoImportPaths = stringlistflag.Flag{}
	withDiscovery    = flag.Bool(
		"discovery", true,
		"generate pb.discovery.go file")
	descFile = flag.String(
		"desc",
		"",
		"Writes a FileDescriptorSet file containing all the the .proto files and their transitive dependencies",
	)
)

// Well-known Google proto packages -> go packages they are implemented in.
var googlePackages = map[string]string{
	"google/protobuf/any.proto":        "github.com/golang/protobuf/ptypes/any",
	"google/protobuf/descriptor.proto": "github.com/golang/protobuf/protoc-gen-go/descriptor",
	"google/protobuf/duration.proto":   "github.com/golang/protobuf/ptypes/duration",
	"google/protobuf/empty.proto":      "github.com/golang/protobuf/ptypes/empty",
	"google/protobuf/struct.proto":     "github.com/golang/protobuf/ptypes/struct",
	"google/protobuf/timestamp.proto":  "github.com/golang/protobuf/ptypes/timestamp",
	"google/protobuf/wrappers.proto":   "github.com/golang/protobuf/ptypes/wrappers",

	"google/rpc/code.proto":          "google.golang.org/genproto/googleapis/rpc/code",
	"google/rpc/error_details.proto": "google.golang.org/genproto/googleapis/rpc/errdetails",
	"google/rpc/status.proto":        "google.golang.org/genproto/googleapis/rpc/status",
}

// compile runs protoc on protoFiles. protoFiles must be relative to dir.
func compile(c context.Context, gopath, importPaths, protoFiles []string, dir, descSetOut string) (outDir string, err error) {
	// make it absolute to find in $GOPATH and because protoc wants paths
	// to be under proto paths.
	if dir, err = filepath.Abs(dir); err != nil {
		return "", err
	}

	// By default place go files in CWD,
	// unless proto files are under a $GOPATH/src.
	goOut := "."

	// Combine user-defined proto paths with $GOPATH/src.
	allProtoPaths := make([]string, 0, len(importPaths)+len(gopath)+1)
	for _, p := range importPaths {
		if p, err = filepath.Abs(p); err != nil {
			return "", err
		}
		allProtoPaths = append(allProtoPaths, p)
	}
	for _, p := range gopath {
		path := filepath.Join(p, "src")
		if info, err := os.Stat(path); os.IsNotExist(err) || !info.IsDir() {
			continue
		} else if err != nil {
			return "", err
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
			outDir = filepath.Join(goOut, dir[len(p):])
			break
		}
	}
	if outDir == "" {
		return "", fmt.Errorf("proto files are neither under $GOPATH/src nor -proto-path")
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
	for k, v := range googlePackages {
		params = append(params, fmt.Sprintf("M%s=%s", k, v))
	}
	params = append(params, "plugins=grpc")
	args = append(args, fmt.Sprintf("--go_out=%s:%s", strings.Join(params, ","), goOut))

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
	logging.Infof(c, "protoc %s", strings.Join(args, " "))
	protoc := exec.Command("protoc", args...)
	protoc.Stdout = os.Stdout
	protoc.Stderr = os.Stderr
	return outDir, protoc.Run()
}

func run(c context.Context, goPath []string, dir string) error {
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
		tmpDir, err := ioutil.TempDir("", "")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		descPath = filepath.Join(tmpDir, "package.desc")
	}

	outDir, err := compile(c, goPath, protoImportPaths, protoFiles, dir, descPath)
	if err != nil {
		return err
	}

	// Transform .go files
	var goPkg, protoPkg string
	for _, p := range protoFiles {
		goFile := filepath.Join(outDir, strings.TrimSuffix(p, ".proto")+".pb.go")
		var t transformer
		if err := t.transformGoFile(goFile); err != nil {
			return fmt.Errorf("could not transform %s: %s", goFile, err)
		}

		if protoPkg == "" && len(t.services) > 0 {
			protoPkg = t.services[0].protoPackageName
		}
		if goPkg == "" {
			goPkg = t.PackageName
		}

		if strings.HasSuffix(p, "_test.proto") {
			newName := strings.TrimSuffix(goFile, ".go") + "_test.go"
			if err := os.Rename(goFile, newName); err != nil {
				return err
			}
		}
	}
	if *withDiscovery && goPkg != "" && protoPkg != "" {
		// Generate pb.prpc.go
		discoveryFile := "pb.discovery.go"
		if err := genDiscoveryFile(c, filepath.Join(outDir, discoveryFile), descPath, protoPkg, goPkg); err != nil {
			return err
		}
	}

	return nil
}

func setupLogging(c context.Context) context.Context {
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
