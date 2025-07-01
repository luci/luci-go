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

// Package protoc contains helpers for running `protoc` using protos files
// stored in the Go source tree.
package protoc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
)

// CompileParams are passed to Compile.
type CompileParams struct {
	Inputs                 *StagedInputs     // a staging directory with proto files
	OutputDescriptorSet    string            // a path to write the descriptor set to
	GoEnabled              bool              // true to use protoc-gen-go plugin
	GoPackageMap           map[string]string // maps a proto path to a go package name
	GoDeprecatedGRPCPlugin bool              // true to use deprecated grpc protoc-gen-go plugin
	GoGRPCEnabled          bool              // true to use protoc-gen-go-grpc
	GoPGVEnabled           bool              // enable protoc-gen-validate support
	PrependBinPath         []string          // extra directories to prepend to PATH (for discovering plugins in)
}

// Compile runs protoc over staged inputs.
func Compile(ctx context.Context, p *CompileParams) error {
	// Common protoc arguments.
	args := []string{
		"--descriptor_set_out=" + p.OutputDescriptorSet,
		"--include_imports",
		"--include_source_info",
	}
	for _, path := range p.Inputs.Paths {
		args = append(args, "--proto_path="+path)
	}

	if p.GoEnabled {
		// protoc-gen-go plugin arguments.
		var params []string
		for k, v := range p.GoPackageMap {
			params = append(params, fmt.Sprintf("M%s=%s", k, v))
		}
		sort.Strings(params)
		if p.GoDeprecatedGRPCPlugin {
			params = append(params, "plugins=grpc")
		}
		args = append(args, fmt.Sprintf("--go_out=%s:%s", strings.Join(params, ","), p.Inputs.OutputDir))

		// protoc-gen-go-grpc plugin arguments.
		if p.GoGRPCEnabled {
			args = append(args, fmt.Sprintf("--go-grpc_out=%s", p.Inputs.OutputDir))
		}

		if p.GoPGVEnabled {
			args = append(args, fmt.Sprintf("--validate_out=lang=go:%s", p.Inputs.OutputDir))
		}
	}

	// protoc searches import paths purely lexicographically. Since we pass
	// import paths as absolute, we must use absolute paths for all input protos
	// as well, otherwise protoc gets confused.
	for _, f := range p.Inputs.ProtoFiles {
		args = append(args, path.Join(p.Inputs.InputDir, f))
	}

	logging.Debugf(ctx, "protoc %s", strings.Join(args, " "))
	protoc := exec.Command("protoc", args...)
	protoc.Stdout = os.Stdout
	protoc.Stderr = os.Stderr

	if len(p.PrependBinPath) != 0 {
		env := environ.System()
		env.Set("PATH", fmt.Sprintf("%s%s%s",
			strings.Join(p.PrependBinPath, string(os.PathListSeparator)),
			string(os.PathListSeparator),
			env.Get("PATH"),
		))
		protoc.Env = env.Sorted()
	}

	return protoc.Run()
}
