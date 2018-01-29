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
	"fmt"
	"io"
	"strings"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"

	"go.chromium.org/luci/common/data/text/indented"
	"go.chromium.org/luci/common/proto/google/descutil"
)

// printer prints a proto3 definition from a description.
// Does not support options.
type printer struct {
	file           *descriptor.FileDescriptorProto
	sourceCodeInfo map[interface{}]*descriptor.SourceCodeInfo_Location

	Out indented.Writer

	// Err is not nil if writing to Out failed.
	Err error
}

func newPrinter(out io.Writer) *printer {
	return &printer{Out: indented.Writer{Writer: out}}
}

// SetFile specifies the file containing the descriptors being printed.
// Used to relativize names and print comments.
func (p *printer) SetFile(f *descriptor.FileDescriptorProto) error {
	p.file = f
	var err error
	p.sourceCodeInfo, err = descutil.IndexSourceCodeInfo(f)
	return err
}

// Printf prints to p.Out unless there was an error.
func (p *printer) Printf(format string, a ...interface{}) {
	if p.Err == nil {
		_, p.Err = fmt.Fprintf(&p.Out, format, a...)
	}
}

// Package prints package declaration.
func (p *printer) Package(name string) {
	p.Printf("package %s;\n", name)
}

// open prints a string, followed by " {\n" and increases indentation level.
// Returns a function that decreases indentation level and closes the brace.
// Usage: defer open("package x")()
func (p *printer) open(format string, a ...interface{}) func() {
	p.Printf(format, a...)
	p.Printf(" {\n")
	p.Out.Level++
	return func() {
		p.Out.Level--
		p.Printf("}\n")
	}
}

// MaybeLeadingComments prints leading comments of the descriptor proto
// if found.
func (p *printer) MaybeLeadingComments(ptr interface{}) {
	comments := p.sourceCodeInfo[ptr].GetLeadingComments()
	// print comments, but insert "//" before each newline.
	for len(comments) > 0 {
		var toPrint string
		if lineEnd := strings.Index(comments, "\n"); lineEnd >= 0 {
			toPrint = comments[:lineEnd+1] // includes newline
			comments = comments[lineEnd+1:]
		} else {
			// actually this does not happen, because comments always end with
			// newline, but just in case.
			toPrint = comments + "\n"
			comments = ""
		}
		p.Printf("//%s", toPrint)
	}
}

// shorten removes leading "." and trims package name if it matches p.file.
func (p *printer) shorten(name string) string {
	name = strings.TrimPrefix(name, ".")
	if p.file.GetPackage() != "" {
		name = strings.TrimPrefix(name, p.file.GetPackage()+".")
	}
	return name
}

// Service prints a service definition.
// If methodIndex != -1, only one method is printed.
// If serviceIndex != -1, leading comments are printed if found.
func (p *printer) Service(service *descriptor.ServiceDescriptorProto, methodIndex int) {
	p.MaybeLeadingComments(service)
	defer p.open("service %s", service.GetName())()

	if methodIndex < 0 {
		for i := range service.Method {
			p.Method(service.Method[i])
		}
	} else {
		p.Method(service.Method[methodIndex])
		if len(service.Method) > 1 {
			p.Printf("// other methods were omitted.\n")
		}
	}
}

// Method prints a service method definition.
func (p *printer) Method(method *descriptor.MethodDescriptorProto) {
	p.MaybeLeadingComments(method)
	p.Printf(
		"rpc %s(%s) returns (%s) {};\n",
		method.GetName(),
		p.shorten(method.GetInputType()),
		p.shorten(method.GetOutputType()),
	)
}

var fieldTypeName = map[descriptor.FieldDescriptorProto_Type]string{
	descriptor.FieldDescriptorProto_TYPE_DOUBLE:   "double",
	descriptor.FieldDescriptorProto_TYPE_FLOAT:    "float",
	descriptor.FieldDescriptorProto_TYPE_INT64:    "int64",
	descriptor.FieldDescriptorProto_TYPE_UINT64:   "uint64",
	descriptor.FieldDescriptorProto_TYPE_INT32:    "int32",
	descriptor.FieldDescriptorProto_TYPE_FIXED64:  "fixed64",
	descriptor.FieldDescriptorProto_TYPE_FIXED32:  "fixed32",
	descriptor.FieldDescriptorProto_TYPE_BOOL:     "bool",
	descriptor.FieldDescriptorProto_TYPE_STRING:   "string",
	descriptor.FieldDescriptorProto_TYPE_BYTES:    "bytes",
	descriptor.FieldDescriptorProto_TYPE_UINT32:   "uint32",
	descriptor.FieldDescriptorProto_TYPE_SFIXED32: "sfixed32",
	descriptor.FieldDescriptorProto_TYPE_SFIXED64: "sfixed64",
	descriptor.FieldDescriptorProto_TYPE_SINT32:   "sint32",
	descriptor.FieldDescriptorProto_TYPE_SINT64:   "sint64",
}

// Field prints a field definition.
func (p *printer) Field(field *descriptor.FieldDescriptorProto) {
	p.MaybeLeadingComments(field)
	if descutil.Repeated(field) {
		p.Printf("repeated ")
	}

	typeName := fieldTypeName[field.GetType()]
	if typeName == "" {
		typeName = p.shorten(field.GetTypeName())
	}
	if typeName == "" {
		typeName = "<unsupported type>"
	}
	p.Printf("%s %s = %d;\n", typeName, field.GetName(), field.GetNumber())
}

// Message prints a message definition.
func (p *printer) Message(msg *descriptor.DescriptorProto) {
	p.MaybeLeadingComments(msg)
	defer p.open("message %s", msg.GetName())()

	for i := range msg.GetOneofDecl() {
		p.OneOf(msg, i)
	}

	for i, f := range msg.Field {
		if f.OneofIndex == nil {
			p.Field(msg.Field[i])
		}
	}
}

// OneOf prints a oneof definition.
func (p *printer) OneOf(msg *descriptor.DescriptorProto, oneOfIndex int) {
	of := msg.GetOneofDecl()[oneOfIndex]
	p.MaybeLeadingComments(of)
	defer p.open("oneof %s", of.GetName())()

	for i, f := range msg.Field {
		if f.OneofIndex != nil && int(f.GetOneofIndex()) == oneOfIndex {
			p.Field(msg.Field[i])
		}
	}
}

// Enum prints an enum definition.
func (p *printer) Enum(enum *descriptor.EnumDescriptorProto) {
	p.MaybeLeadingComments(enum)
	defer p.open("enum %s", enum.GetName())()

	for _, v := range enum.Value {
		p.EnumValue(v)
	}
}

// EnumValue prints an enum value definition.
func (p *printer) EnumValue(v *descriptor.EnumValueDescriptorProto) {
	p.MaybeLeadingComments(v)
	p.Printf("%s = %d;\n", v.GetName(), v.GetNumber())
}
