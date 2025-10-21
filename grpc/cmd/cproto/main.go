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
	"go/ast"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/proto/protoc"
	"go.chromium.org/luci/common/proto/protocgengo"
	"go.chromium.org/luci/common/system/exitcode"
)

const protocGenValidatePkg = "github.com/envoyproxy/protoc-gen-validate"

var (
	verbose          = flag.Bool("verbose", false, "print debug messages to stderr")
	protoImportPaths = stringlistflag.Flag{}
	goModules        = stringlistflag.Flag{}
	goRootModules    = stringlistflag.Flag{}
	pathMap          = stringmapflag.Value{}
	withDiscovery    = flag.Bool(
		"discovery", true,
		"generate pb.discovery.go file")
	discoveryGoPkg = flag.String(
		"discovery-go-pkg",
		"",
		"a go package to put pb.discovery.go file into in case go_package proto option is ambiguous (accepts the same format as go_package proto option)",
	)
	descFile = flag.String(
		"desc",
		"",
		"write FileDescriptorSet file containing all the .proto files and their transitive dependencies",
	)
	disableGRPC = flag.Bool(
		"disable-grpc", false,
		"disable grpc and prpc stubs generation, implies -discovery=false",
	)
	useGRPCPlugin = flag.Bool(
		"use-grpc-plugin", false,
		"use protoc-gen-go-grpc to generate gRPC stubs instead of protoc-gen-go",
	)
	enablePGV = flag.Bool(
		"enable-pgv", false,
		"enable protoc-gen-validate generation. Makes 'validate/validate.proto' and annotations available.",
	)
	useModernProtocGenGo = flag.Bool(
		"use-modern-protoc-gen-go", false,
		"use pinned google.golang.org/protobuf/cmd/protoc-gen-go for generation, implies -use-grpc-plugin",
	)
	useAncientProtocGenGo = flag.Bool(
		"use-ancient-protoc-gen-go", false,
		"use pinned github.com/golang/protobuf/protoc-gen-go for generation",
	)
	moveGrpcIntoSubpackage = flag.Bool(
		"move-grpc-into-subpackage", false,
		"move generated _grpc.pb.go into a .../grpcpb subpackage",
	)
)

func run(ctx context.Context, inputDir string) error {
	if *enablePGV {
		needAddPkg := true
		for _, mod := range goRootModules {
			if mod == protocGenValidatePkg {
				needAddPkg = false
				break
			}
		}
		if needAddPkg {
			logging.Infof(ctx, "adding -go-root-module %s", goRootModules)
			goRootModules = append(goRootModules, protocGenValidatePkg)
		}
	}

	if *useModernProtocGenGo && !*useGRPCPlugin {
		logging.Infof(ctx, "implicitly enabling -use-grpc-plugin since it is required when using modern protoc-gen-go")
		*useGRPCPlugin = true
	}

	if *moveGrpcIntoSubpackage {
		if *disableGRPC {
			return errors.Fmt("-move-grpc-into-subpackage requires grpc generation enabled")
		}
		if !*useGRPCPlugin {
			return errors.Fmt("-move-grpc-into-subpackage requires -use-grpc-plugin")
		}
	}

	// Stage all requested Go modules under a single root.
	inputs, err := protoc.StageGoInputs(ctx, inputDir, goModules, goRootModules, protoImportPaths)
	if err != nil {
		return err
	}
	defer inputs.Cleanup()

	// Prep a path to the generated descriptors file.
	descPath := *descFile
	if descPath == "" {
		tmpDir, err := os.MkdirTemp("", "")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		descPath = filepath.Join(tmpDir, "package.desc")
	}

	// Prepare a path with a modern version of protoc-gen-go.
	var prependBinPath []string
	if *useModernProtocGenGo || *useAncientProtocGenGo {
		var ver protocgengo.Version
		switch {
		case *useModernProtocGenGo && *useAncientProtocGenGo:
			return errors.Fmt("cannot -use-modern-protoc-gen-go and -use-ancient-protoc-gen-go at the same time")
		case *useModernProtocGenGo:
			ver = protocgengo.ModernVersion
		case *useAncientProtocGenGo:
			ver = protocgengo.AncientVersion
		}
		path, err := protocgengo.Bootstrap(ver)
		if err != nil {
			return errors.Fmt("could not setup protoc-gen-go: %w", err)
		}
		defer os.RemoveAll(path)
		prependBinPath = []string{path}
	}

	// Compile all .proto files.
	err = protoc.Compile(ctx, &protoc.CompileParams{
		Inputs:                 inputs,
		OutputDescriptorSet:    descPath,
		GoEnabled:              true,
		GoPackageMap:           pathMap,
		GoDeprecatedGRPCPlugin: !*disableGRPC && !*useGRPCPlugin,
		GoGRPCEnabled:          !*disableGRPC && *useGRPCPlugin,
		GoPGVEnabled:           *enablePGV,
		PrependBinPath:         prependBinPath,
	})
	if err != nil {
		return err
	}

	// protoc-gen-go puts generated files based on go_package option, rooting them
	// in the inputs.OutputDir. We can't generally guess the Go package name just
	// based on proto file names, but we can extract it from the generated
	// descriptor.
	//
	// Doc:
	// https://developers.google.com/protocol-buffers/docs/reference/go-generated
	descSet, rawDesc, err := loadDescriptorSet(descPath)
	if err != nil {
		return errors.Fmt("failed to load the descriptor set with generated files: %w", err)
	}

	generatedDesc := make([]*descriptorpb.FileDescriptorProto, 0, len(inputs.ProtoFiles))

	// Since we use --include_imports, there may be a lot of descriptors in the
	// set. Visit only ones we care about.
	for _, protoFile := range inputs.ProtoFiles {
		fileDesc := descSet[path.Join(inputs.ProtoPackage, protoFile)]
		if fileDesc == nil {
			return errors.Fmt("descriptor for %q is unexpectedly absent", protoFile)
		}
		generatedDesc = append(generatedDesc, fileDesc)

		// "go_package" option is required now.
		goPackage := fileDesc.Options.GetGoPackage()
		if goPackage == "" {
			return errors.Fmt("file %q has no go_package option set, it is required", protoFile)
		}
		// Convert e.g. "foo/bar;pkgname" => "foo/bar".
		if idx := strings.LastIndex(goPackage, ";"); idx != -1 {
			goPackage = goPackage[:idx]
		}

		// A file that protoc must have generated for us.
		goFile := filepath.Join(
			inputs.OutputDir,
			filepath.FromSlash(goPackage),
			strings.TrimSuffix(protoFile, ".proto")+".pb.go",
		)
		if _, err := os.Stat(goFile); err != nil {
			return errors.Fmt("could not find *.pb.go file generated from %q, is go_package option correct?", protoFile)
		}

		// Transform .go files by adding pRPC stubs after gPRC stubs. Code generated
		// by protoc-gen-go-grpc plugin doesn't need this, since it uses interfaces
		// in the generated code (that pRPC implements) instead of concrete gRPC
		// types.
		if !*disableGRPC && !*useGRPCPlugin {
			var t transformer
			if err := t.transformGoFile(goFile); err != nil {
				return errors.Fmt("could not transform %q: %w", goFile, err)
			}
		}

		// _test.proto's should go into the test package.
		if strings.HasSuffix(protoFile, "_test.proto") {
			newName := strings.TrimSuffix(goFile, ".go") + "_test.go"
			if err := os.Rename(goFile, newName); err != nil {
				return err
			}
		}

		// Move _grpc.pb.go into a subdirectory if asked to.
		if *moveGrpcIntoSubpackage && len(fileDesc.Service) != 0 {
			grpcFile := strings.TrimSuffix(goFile, ".pb.go") + "_grpc.pb.go"
			if _, err := os.Stat(grpcFile); err != nil {
				return errors.Fmt("could not find *_grpc.pb.go file generated from %q", protoFile)
			}
			logging.Debugf(ctx, "Moving %q into a subpackage", grpcFile)
			if err := relocateGrpcPbGo(grpcFile, goPackage); err != nil {
				return errors.Fmt("failed to move %q into a subpackage: %w", grpcFile, err)
			}
		}
	}

	if !*disableGRPC && *withDiscovery && hasServices(generatedDesc) {
		// If -discovery-go-pkg is not given, choose "go_package" option as long as
		// it is non-ambiguous.
		discoveryPkgSpec := *discoveryGoPkg
		if discoveryPkgSpec == "" {
			goPackageOpts := stringset.New(0)
			for _, desc := range generatedDesc {
				goPackageOpts.Add(desc.Options.GetGoPackage())
			}
			if goPackageOpts.Len() != 1 {
				return errors.Fmt("cannot generate pb.discovery.go: generated *.pb.go files "+
					"are in multiple packages %v, use -discovery-go-pkg flag to specify a package to put the discovery file into",
					goPackageOpts.ToSortedSlice(),
				)
			}
			discoveryPkgSpec = goPackageOpts.ToSlice()[0]
		}

		var goPkgPath string // e.g. "go.chromium.org/luci/api/v1"
		var goPkgName string // e.g. "apipb"
		switch parts := strings.Split(discoveryPkgSpec, ";"); {
		case len(parts) == 1:
			goPkgPath = discoveryPkgSpec
			goPkgName = path.Base(discoveryPkgSpec)
		case len(parts) == 2:
			goPkgPath = parts[0]
			goPkgName = parts[1]
		default:
			return errors.Fmt("unrecognized format of go_package option spec %q", discoveryPkgSpec)
		}

		out := filepath.Join(
			inputs.OutputDir,
			filepath.FromSlash(goPkgPath),
			"pb.discovery.go",
		)
		if err := genDiscoveryFile(out, goPkgPath, goPkgName, generatedDesc, rawDesc); err != nil {
			return err
		}
	}

	return nil
}

// loadDescriptorSet reads and parses FileDescriptorSet proto.
//
// Returns it as a map: *.proto path in the registry => FileDescriptorProto,
// as well as a raw byte blob.
func loadDescriptorSet(path string) (map[string]*descriptorpb.FileDescriptorProto, []byte, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(blob, set); err != nil {
		return nil, nil, err
	}
	mapping := make(map[string]*descriptorpb.FileDescriptorProto, len(set.File))
	for _, f := range set.File {
		mapping[f.GetName()] = f
	}
	return mapping, blob, nil
}

func relocateGrpcPbGo(grpcFile, goPackage string) error {
	orig, err := os.ReadFile(grpcFile)
	if err != nil {
		return err
	}

	transformed, err := transformGoFile(grpcFile, orig, func(f *ast.File) error {
		// Rename the package, e.g. "stuffpb" => "stuffgrpcpb".
		f.Name.Name = strings.TrimSuffix(f.Name.Name, "pb") + "grpcpb"
		// Add a dot import to allow proto messages (for requests and responses)
		// to be referenced unqualified. Otherwise we'd need to find and replace all
		// their occurrences, which can be non-trivial.
		spec := &ast.ImportSpec{
			Name: ast.NewIdent("."),
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: `"` + goPackage + `"`,
			},
		}
		f.Decls = append([]ast.Decl{&ast.GenDecl{
			Tok:   token.IMPORT,
			Specs: []ast.Spec{spec},
		}}, f.Decls...)
		return nil
	})
	if err != nil {
		return err
	}

	destDir := filepath.Join(filepath.Dir(grpcFile), "grpcpb")
	if err := os.MkdirAll(destDir, 0777); err != nil {
		return err
	}
	destPath := filepath.Join(destDir, filepath.Base(grpcFile))
	if err := os.WriteFile(destPath, transformed, 0666); err != nil {
		return err
	}

	return os.Remove(grpcFile)
}

func setupLogging(ctx context.Context) context.Context {
	lvl := logging.Warning
	if *verbose {
		lvl = logging.Debug
	}
	return logging.SetLevel(gologger.StdConfig.Use(ctx), lvl)
}

func usage() {
	fmt.Fprintln(os.Stderr,
		`Compiles all .proto files in a directory to .go with grpc+prpc+validate support.
usage: cproto [flags] [dir]

This also has support for github.com/envoyproxy/protoc-gen-validate. Have your
proto files import "validate/validate.proto" and then add '-enable-pgv' to your
cproto invocation to generate Validate() calls for your proto library.

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
		&goRootModules,
		"go-root-module",
		"make protos relative to the root of the given module available in proto import path. "+
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
