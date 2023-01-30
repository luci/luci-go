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

package printer

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	txtpb "github.com/protocolbuffers/txtpbfmt/parser"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/data/text/indented"
	"go.chromium.org/luci/common/proto/google/descutil"
)

// Printer prints a proto3 definition from a description.
type Printer struct {
	file           *descriptorpb.FileDescriptorProto
	sourceCodeInfo map[interface{}]*descriptorpb.SourceCodeInfo_Location

	Out indented.Writer

	// Err is not nil if writing to Out failed.
	Err error
}

// NewPrinter creates a new Printer which will output protobuf definition text
// (i.e. ".proto" file) to the given writer.
func NewPrinter(out io.Writer) *Printer {
	return &Printer{Out: indented.Writer{Writer: out}}
}

// SetFile specifies the file containing the descriptors being printed.
// Used to relativize names and print comments.
func (p *Printer) SetFile(f *descriptorpb.FileDescriptorProto) error {
	p.file = f
	var err error
	p.sourceCodeInfo, err = descutil.IndexSourceCodeInfo(f)
	return err
}

// Printf prints to p.Out unless there was an error.
func (p *Printer) Printf(format string, a ...interface{}) {
	if p.Err == nil {
		_, p.Err = fmt.Fprintf(&p.Out, format, a...)
	}
}

// Package prints package declaration.
func (p *Printer) Package(name string) {
	p.Printf("package %s;\n", name)
}

// open prints a string, followed by " {\n" and increases indentation level.
// Returns a function that decreases indentation level and closes the brace
// followed by a newline.
// Usage: defer open("package x")()
func (p *Printer) open(format string, a ...interface{}) func() {
	p.Printf(format, a...)
	p.Printf(" {\n")
	p.Out.Level++
	return func() {
		p.Out.Level--
		p.Printf("}\n")
	}
}

// MaybeLeadingComments prints leading comments of the descriptorpb proto
// if found.
func (p *Printer) MaybeLeadingComments(ptr interface{}) {
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

// AppendLeadingComments allows adding additional leading comments to any printable
// descriptorpb object associated with this printer.
//
// Each line will be prepended with " " and appended with "\n".
//
// e.g.
//
//	p := NewPrinter(os.Stdout)
//	p.AppendLeadingComments(protodesc.ToDescriptorProto(myMsg.ProtoReflect()), []string{
//	  "This is a line.",
//	  "This is the next line.",
//	})
func (p *Printer) AppendLeadingComments(ptr interface{}, lines []string) {
	loc, ok := p.sourceCodeInfo[ptr]
	if !ok {
		loc = &descriptorpb.SourceCodeInfo_Location{}
		p.sourceCodeInfo[ptr] = loc
	}
	bld := strings.Builder{}
	for _, line := range lines {
		bld.WriteRune(' ')
		bld.WriteString(line)
		bld.WriteRune('\n')
	}
	comments := loc.GetLeadingComments() + bld.String()
	loc.LeadingComments = &comments
}

// shorten removes leading "." and trims package name if it matches p.file.
func (p *Printer) shorten(name string) string {
	name = strings.TrimPrefix(name, ".")
	if p.file.GetPackage() != "" {
		name = strings.TrimPrefix(name, p.file.GetPackage()+".")
	}
	return name
}

// Service prints a service definition.
// If methodIndex != -1, only one method is printed.
// If serviceIndex != -1, leading comments are printed if found.
func (p *Printer) Service(service *descriptorpb.ServiceDescriptorProto, methodIndex int) {
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
func (p *Printer) Method(method *descriptorpb.MethodDescriptorProto) {
	p.MaybeLeadingComments(method)
	p.Printf(
		"rpc %s(%s) returns (%s) {};\n",
		method.GetName(),
		p.shorten(method.GetInputType()),
		p.shorten(method.GetOutputType()),
	)
}

var fieldTypeName = map[descriptorpb.FieldDescriptorProto_Type]string{
	descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:   "double",
	descriptorpb.FieldDescriptorProto_TYPE_FLOAT:    "float",
	descriptorpb.FieldDescriptorProto_TYPE_INT64:    "int64",
	descriptorpb.FieldDescriptorProto_TYPE_UINT64:   "uint64",
	descriptorpb.FieldDescriptorProto_TYPE_INT32:    "int32",
	descriptorpb.FieldDescriptorProto_TYPE_FIXED64:  "fixed64",
	descriptorpb.FieldDescriptorProto_TYPE_FIXED32:  "fixed32",
	descriptorpb.FieldDescriptorProto_TYPE_BOOL:     "bool",
	descriptorpb.FieldDescriptorProto_TYPE_STRING:   "string",
	descriptorpb.FieldDescriptorProto_TYPE_BYTES:    "bytes",
	descriptorpb.FieldDescriptorProto_TYPE_UINT32:   "uint32",
	descriptorpb.FieldDescriptorProto_TYPE_SFIXED32: "sfixed32",
	descriptorpb.FieldDescriptorProto_TYPE_SFIXED64: "sfixed64",
	descriptorpb.FieldDescriptorProto_TYPE_SINT32:   "sint32",
	descriptorpb.FieldDescriptorProto_TYPE_SINT64:   "sint64",
}

// Field prints a field definition.
func (p *Printer) Field(field *descriptorpb.FieldDescriptorProto) {
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
	p.Printf("%s %s = %d", typeName, field.GetName(), field.GetNumber())

	p.fieldOptions(field)

	p.Printf(";\n")
}

// converts snake_case to camelCase.
func camel(snakeCase string) string {
	prev := 'x'
	return strings.Map(
		func(r rune) rune {
			if prev == '_' {
				prev = r
				return unicode.ToTitle(r)
			}
			prev = r
			if r == '_' {
				return -1
			}
			return r
		}, snakeCase)
}

func (p *Printer) optionValue(ed protoreflect.EnumDescriptor, v protoreflect.Value) {
	switch x := v.Interface().(type) {
	case bool:
		p.Printf(" %t", x)
	case int32, int64, uint32, uint64:
		p.Printf(" %d", v.Interface())
	case float32, float64:
		p.Printf(" %f", v.Interface())
	case string, []byte:
		p.Printf(" %q", v.Interface())
	case protoreflect.EnumNumber:
		p.Printf(" %s", ed.Values().ByNumber(x).Name())
	case protoreflect.Message:
		p.Printf(" {\n")
		p.Out.Level++
		defer func() {
			p.Out.Level--
			p.Printf("}")
		}()

		data, err := prototext.MarshalOptions{Indent: "\t"}.Marshal(x.Interface())
		if err != nil {
			panic(err)
		}
		// ensure textproto output is stable.
		data, err = txtpb.Format(data)
		if err != nil {
			panic(err)
		}

		p.Printf("%s", data)
	default:
		panic("unknown protoreflect.Value type")
	}

	return
}

type optField struct {
	fieldNum       protoreflect.FieldNumber
	renderedName   string
	enumDescriptor protoreflect.EnumDescriptor
	val            protoreflect.Value
}

type optFields []optField

func (p *Printer) collectOptions(m protoreflect.ProtoMessage, extra optFields) optFields {
	toPrint := append(optFields{}, extra...)

	m.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		renderedName := fd.TextName()
		if fd.IsExtension() {
			renderedName = fmt.Sprintf("(%s)", p.shorten(string(fd.FullName())))
		}
		toPrint = append(toPrint, optField{fd.Number(), renderedName, fd.Enum(), v})
		return true
	})
	sort.Slice(toPrint, func(i, j int) bool {
		return toPrint[i].fieldNum < toPrint[j].fieldNum
	})
	return toPrint
}

func (of optFields) write(p *Printer, prefix string, afterEach func(last bool)) {
	for i, f := range of {
		p.Printf("%s%s =", prefix, f.renderedName)
		p.optionValue(f.enumDescriptor, f.val)
		if afterEach != nil {
			afterEach(i == len(of)-1)
		}
	}
}

func (p *Printer) fieldOptions(field *descriptorpb.FieldDescriptorProto) {
	var extra optFields
	if field.GetJsonName() != camel(field.GetName()) {
		extra = optFields{{
			0, "json_name", nil, protoreflect.ValueOfString(field.GetJsonName()),
		}}
	}
	toPrint := p.collectOptions(field.Options, extra)

	if len(toPrint) == 0 {
		return
	}
	if len(toPrint) == 1 {
		p.Printf(" [")
		toPrint.write(p, "", nil)
		p.Printf("]")
		return
	}

	p.Printf(" [\n")
	p.Out.Level++
	defer func() {
		p.Out.Level--
		p.Printf("]")
	}()

	nl := func(hasNext bool) {
		if hasNext {
			p.Printf(",\n")
		} else {
			p.Printf("\n")
		}
	}

	toPrint.write(p, "", func(last bool) {
		nl(!last)
	})
}

// Message prints a message definition.
func (p *Printer) Message(msg *descriptorpb.DescriptorProto) {
	p.MaybeLeadingComments(msg)
	defer p.open("message %s", msg.GetName())()

	p.collectOptions(msg.Options, nil).write(p, "option ", func(last bool) {
		p.Printf(";\n")
	})

	for _, name := range msg.ReservedName {
		p.Printf("reserved %q;\n", name)
	}

	for _, rng := range msg.ReservedRange {
		if rng.GetStart() == rng.GetEnd()-1 {
			p.Printf("reserved %d;\n", rng.GetStart())
		} else {
			p.Printf("reserved %d to %d;\n", rng.GetStart(), rng.GetEnd())
		}
	}

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
func (p *Printer) OneOf(msg *descriptorpb.DescriptorProto, oneOfIndex int) {
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
func (p *Printer) Enum(enum *descriptorpb.EnumDescriptorProto) {
	p.MaybeLeadingComments(enum)
	defer p.open("enum %s", enum.GetName())()

	for _, v := range enum.Value {
		p.EnumValue(v)
	}
}

// EnumValue prints an enum value definition.
func (p *Printer) EnumValue(v *descriptorpb.EnumValueDescriptorProto) {
	p.MaybeLeadingComments(v)
	p.Printf("%s = %d;\n", v.GetName(), v.GetNumber())
}
