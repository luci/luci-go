// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/system/exitcode"
)

var (
	verbose    = flag.Bool("verbose", false, "print debug messages to stderr")
	importPath = flag.String(
		"import-path",
		"",
		"Override the generated Go import path with this value.")
	withDiscovery = flag.Bool(
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
}

// compile runs protoc on protoFiles. protoFiles must be relative to dir.
func compile(c context.Context, gopath, protoFiles []string, dir, descSetOut string) error {
	args := []string{
		"--descriptor_set_out=" + descSetOut,
		"--include_imports",
		"--include_source_info",
	}

	// make it absolute to find in $GOPATH and because protoc wants paths
	// to be under proto paths.
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	currentGoPath := ""
	for _, p := range gopath {
		path := filepath.Join(p, "src")
		if info, err := os.Stat(path); os.IsNotExist(err) || !info.IsDir() {
			continue
		} else if err != nil {
			return err
		}
		args = append(args, "--proto_path="+path)
		if strings.HasPrefix(dir, path) {
			currentGoPath = path
		}
	}

	if currentGoPath == "" {
		return fmt.Errorf("directory %q is not inside current $GOPATH", dir)
	}

	var params []string
	for k, v := range googlePackages {
		params = append(params, fmt.Sprintf("M%s=%s", k, v))
	}

	if p := *importPath; p != "" {
		params = append(params, fmt.Sprintf("import_path=%s", p))
	}
	params = append(params, "plugins=grpc")
	args = append(args, fmt.Sprintf("--go_out=%s:%s", strings.Join(params, ","), currentGoPath))

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
	return protoc.Run()
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

	if err := compile(c, goPath, protoFiles, dir, descPath); err != nil {
		return err
	}

	// Transform .go files
	var goPkg, protoPkg string
	for _, p := range protoFiles {
		goFile := filepath.Join(dir, strings.TrimSuffix(p, ".proto")+".pb.go")
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
		if err := genDiscoveryFile(c, filepath.Join(dir, discoveryFile), descPath, protoPkg, goPkg); err != nil {
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

Flags:`)
	flag.PrintDefaults()
}

func main() {
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
