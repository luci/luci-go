// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	gol "github.com/op/go-logging"
	"golang.org/x/net/context"
)

var (
	verbose    = flag.Bool("verbose", false, "print debug messages to stderr")
	withGithub = flag.Bool(
		"with-github", true,
		"include $GOPATH/src/github.com in proto search path")
	withGoogleProtobuf = flag.Bool(
		"with-google-protobuf", true,
		"map .proto files in gitub.com/luci/luci-go/common/proto/google to google/protobuf/*.proto")
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

func protoc(c context.Context, protoFiles []string, dir, descSetFile string) error {
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
		"--descriptor_set_out=" + filepath.Join(dir, descSetFile),
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
	if err := protoc(c, protoFiles, dir, "package.desc"); err != nil {
		return err
	}

	// Transform .go files
	for _, p := range protoFiles {
		goFile := strings.TrimSuffix(p, ".proto") + ".pb.go"
		if err := transformGoFile(goFile); err != nil {
			return fmt.Errorf("could not transform %s: %s", goFile, err)
		}
	}

	return nil
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
usage: prpcgen [flags] [dir]

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
