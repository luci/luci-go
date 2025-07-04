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
	"bytes"
	"compress/gzip"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/errors"
)

const (
	discoveryPackagePath = "go.chromium.org/luci/grpc/discovery"
)

// discoveryTmpl is template for generated Go discovery file.
// The result of execution will also be passed through gofmt.
var discoveryTmpl = template.Must(template.New("").Parse(strings.TrimSpace(`
// Code generated by cproto. DO NOT EDIT.

package {{.GoPkg}};

{{if .ImportDiscovery}}
import "go.chromium.org/luci/grpc/discovery"
{{end}}
import "google.golang.org/protobuf/types/descriptorpb"

func init() {
	{{if .ImportDiscovery}}discovery.{{end}}RegisterDescriptorSetCompressed(
		[]string{
			{{range .ServiceNames}}"{{.}}",{{end}}
		},
		{{.CompressedBytes}},
	)
}

// FileDescriptorSet returns a descriptor set for this proto package, which
// includes all defined services, and all transitive dependencies.
//
// Will not return nil.
//
// Do NOT modify the returned descriptor.
func FileDescriptorSet() *descriptorpb.FileDescriptorSet {
	// We just need ONE of the service names to look up the FileDescriptorSet.
	ret, err := {{if .ImportDiscovery}}discovery.{{end}}GetDescriptorSet("{{index .ServiceNames 0 }}")
	if err != nil {
		panic(err)
	}
	return ret
}
`)))

// genDiscoveryFile generates a Go discovery file that calls
// discovery.RegisterDescriptorSetCompressed(serviceNames, compressedDescBytes)
// in an init function.
func genDiscoveryFile(output, goPkg string, desc []*descriptorpb.FileDescriptorProto, raw []byte) error {
	var serviceNames []string
	for _, f := range desc {
		for _, s := range f.Service {
			serviceNames = append(serviceNames, fmt.Sprintf("%s.%s", f.GetPackage(), s.GetName()))
		}
	}
	if len(serviceNames) == 0 {
		// no services, no discovery.
		return nil
	}

	// Get the package name for "package ..." statement: it may be different from
	// the directory name. Note that pkg.ImportPath almost always end up "." here
	// when running in Go Modules mode, so we still need to keep `goPkg` argument.
	pkg, err := build.ImportDir(filepath.Dir(output), 0)
	if err != nil {
		return errors.Fmt("failed to figure out Go package name for %q: %w", output, err)
	}

	compressedDescBytes, err := compress(raw)
	if err != nil {
		return errors.Fmt("failed to compress the descriptor set proto: %w", err)
	}

	var buf bytes.Buffer
	err = discoveryTmpl.Execute(&buf, map[string]any{
		"GoPkg":           pkg.Name,
		"ImportDiscovery": goPkg != discoveryPackagePath,
		"ServiceNames":    serviceNames,
		"CompressedBytes": asByteArray(compressedDescBytes),
	})
	if err != nil {
		return errors.Fmt("failed to execute discovery file template: %w", err)
	}

	src := buf.Bytes()
	formatted, err := gofmt(src)
	if err != nil {
		return errors.Fmt("failed to gofmt the generated discovery file: %w", err)
	}

	return os.WriteFile(output, formatted, 0666)
}

// asByteArray converts blob to a valid []byte Go literal.
func asByteArray(blob []byte) string {
	out := &bytes.Buffer{}
	fmt.Fprintf(out, "[]byte{")
	for i := range blob {
		fmt.Fprintf(out, "%d, ", blob[i])
		if i%14 == 1 {
			fmt.Fprintln(out)
		}
	}
	fmt.Fprintf(out, "}")
	return out.String()
}

// gofmt applies "gofmt -s" to the content of the buffer.
func gofmt(blob []byte) ([]byte, error) {
	out := bytes.Buffer{}
	cmd := exec.Command("gofmt", "-s")
	cmd.Stdin = bytes.NewReader(blob)
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// compress compresses data with gzip.
func compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
