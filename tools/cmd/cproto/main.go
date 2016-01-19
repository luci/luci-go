// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	gol "github.com/op/go-logging"
	"golang.org/x/net/context"
)

var (
	verbose       = flag.Bool("verbose", false, "print debug messages to stderr")
	withDiscovery = flag.Bool(
		"discovery", true,
		"generate pb.discovery.go file")
	withGithub = flag.Bool(
		"with-github", true,
		"include $GOPATH/src/github.com in proto search path")
	withGoogleProtobuf = flag.Bool(
		"with-google-protobuf", true,
		"map .proto files in gitub.com/luci/luci-go/common/proto/google to google/protobuf/*.proto")
	renameToTestGo = flag.Bool(
		"test", false,
		"rename generated files from *.go to *_test.go")
	descFile = flag.String(
		"desc",
		"",
		"Writes a FileDescriptorSet file containing all the the .proto files and their transitive dependencies",
	)
)

func resolveGoogleProtobufPackages(c context.Context) (map[string]string, error) {
	const (
		protoPrefix = "google/protobuf/"
		pkgPath     = "github.com/luci/luci-go/common/proto/google"
	)
	var result = map[string]string{
		protoPrefix + "descriptor.proto": pkgPath + "/descriptor", //snowflake
	}

	pkg, err := build.Import(pkgPath, "", build.FindOnly)
	if err != nil {
		return nil, err
	}
	protoFiles, err := findProtoFiles(pkg.Dir)
	if err != nil {
		return nil, err
	}
	if len(protoFiles) == 0 {
		logging.Warningf(c, "no .proto files in %s", pkg.Dir)
		return nil, nil
	}
	for _, protoFile := range protoFiles {
		result[protoPrefix+filepath.Base(protoFile)] = pkgPath
	}
	return result, nil
}

func protoc(c context.Context, protoFiles []string, dir, descSetOut string) error {
	var params []string

	if *withGoogleProtobuf {
		googlePackages, err := resolveGoogleProtobufPackages(c)
		if err != nil {
			return err
		}
		for k, v := range googlePackages {
			params = append(params, fmt.Sprintf("M%s=%s", k, v))
		}
	}

	params = append(params, "plugins=grpc")
	goOut := fmt.Sprintf("%s:%s", strings.Join(params, ","), dir)

	args := []string{
		"--proto_path=" + dir,
		"--go_out=" + goOut,
		"--descriptor_set_out=" + descSetOut,
		"--include_imports",
		"--include_source_info",
	}

	if *withGithub {
		for _, p := range strings.Split(os.Getenv("GOPATH"), string(filepath.ListSeparator)) {
			path := filepath.Join(p, "src", "github.com")
			if info, err := os.Stat(path); os.IsNotExist(err) || !info.IsDir() {
				continue
			} else if err != nil {
				return err
			}
			args = append(args, "--proto_path="+path)
		}
	}

	args = append(args, protoFiles...)
	logging.Infof(c, "protoc %s", strings.Join(args, " "))
	protoc := exec.Command("protoc", args...)
	protoc.Stdout = os.Stdout
	protoc.Stderr = os.Stderr
	return protoc.Run()
}

func run(c context.Context, dir string) error {
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
	if err := protoc(c, protoFiles, dir, descPath); err != nil {
		return err
	}

	// Transform .go files
	var packageName string
	for _, p := range protoFiles {
		goFile := strings.TrimSuffix(p, ".proto") + ".pb.go"
		var t transformer
		if err := t.transformGoFile(goFile); err != nil {
			return fmt.Errorf("could not transform %s: %s", goFile, err)
		}

		if packageName == "" {
			packageName = t.PackageName
		}

		if *renameToTestGo {
			newName := strings.TrimSuffix(goFile, ".go") + "_test.go"
			if err := os.Rename(goFile, newName); err != nil {
				return err
			}
		}
	}

	if !*withDiscovery {
		return nil
	}
	// Generate pb.prpc.go
	discoveryFile := "pb.discovery.go"
	if *renameToTestGo {
		discoveryFile = "pb.discovery_test.go"
	}
	return genDiscoveryFile(c, filepath.Join(dir, discoveryFile), descPath, packageName)
}

func setupLogging(c context.Context) context.Context {
	logCfg := gologger.LoggerConfig{
		Format: gologger.StandardFormat,
		Out:    os.Stderr,
		Level:  gol.WARNING,
	}
	if *verbose {
		logCfg.Level = gol.DEBUG
	}
	return logCfg.Use(c)
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
	if err := run(c, dir); err != nil {
		exitCode := 1
		if exit, ok := err.(*exec.ExitError); ok {
			if wait, ok := exit.Sys().(syscall.WaitStatus); ok {
				exitCode = wait.ExitStatus()
			}
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(exitCode)
	}
}

func findProtoFiles(dir string) ([]string, error) {
	return filepath.Glob(filepath.Join(dir, "*.proto"))
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
