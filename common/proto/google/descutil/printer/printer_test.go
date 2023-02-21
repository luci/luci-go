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
	"bytes"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/proto/google/descutil"

	// Register proto extensions defined in util.proto.
	_ "go.chromium.org/luci/common/proto/google/descutil/internal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPrinter(t *testing.T) {
	t.Parallel()

	Convey("Printer", t, func() {
		protoFile, err := os.ReadFile("../internal/util.proto")
		So(err, ShouldBeNil)
		protoFileLines := strings.Split(string(protoFile), "\n")

		descFileBytes, err := os.ReadFile("../internal/util.desc")
		So(err, ShouldBeNil)

		var desc descriptorpb.FileDescriptorSet
		err = proto.Unmarshal(descFileBytes, &desc)
		So(err, ShouldBeNil)

		var file *descriptorpb.FileDescriptorProto
		for _, filePb := range desc.File {
			if filePb.GetName() == "go.chromium.org/luci/common/proto/google/descutil/internal/util.proto" {
				file = filePb
				break
			}
		}
		// we must find the util_test.proto file in `desc`
		So(file, ShouldNotBeNil)

		sourceCodeInfo, err := descutil.IndexSourceCodeInfo(file)
		So(err, ShouldBeNil)

		getExpectedDef := func(ptr any, unindent int) string {
			loc := sourceCodeInfo[ptr]
			So(loc, ShouldNotBeNil)
			startLine := loc.Span[0]
			endLine := startLine
			if len(loc.Span) > 3 {
				endLine = loc.Span[2]
			}

			for startLine > 0 && strings.HasPrefix(strings.TrimSpace(protoFileLines[startLine-1]), "//") {
				startLine--
			}

			expected := make([]string, endLine-startLine+1)
			for i := 0; i < len(expected); i++ {
				expected[i] = protoFileLines[int(startLine)+i][unindent:]
			}

			return strings.Join(expected, "\n") + "\n"
		}

		var buf bytes.Buffer
		printer := NewPrinter(&buf)
		So(printer.SetFile(file), ShouldBeNil)

		checkOutput := func(ptr any, unindent int) {
			So(buf.String(), ShouldEqual, getExpectedDef(ptr, unindent))
		}

		Convey("package", func() {
			printer.Package(file.GetPackage())
			checkOutput(file.Package, 0)
		})

		Convey("service", func() {
			for _, s := range file.Service {
				Convey(s.GetName(), func() {
					printer.Service(s, -1)
					checkOutput(s, 0)
				})
			}
		})

		testEnum := func(e *descriptorpb.EnumDescriptorProto, unindent int) {
			Convey(e.GetName(), func() {
				printer.Enum(e)
				checkOutput(e, unindent)
			})
		}

		Convey("enum", func() {
			for _, e := range file.EnumType {
				testEnum(e, 0)
			}
		})

		Convey("message", func() {
			var testMsg func(*descriptorpb.DescriptorProto, int)
			testMsg = func(m *descriptorpb.DescriptorProto, unindent int) {
				Convey(m.GetName(), func() {
					if len(m.NestedType) == 0 && len(m.EnumType) == 0 {
						printer.Message(m)
						checkOutput(m, unindent)
					} else {
						for _, m := range m.NestedType {
							testMsg(m, unindent+1)
						}
						for _, e := range m.EnumType {
							testEnum(e, unindent+1)
						}
					}
				})
			}
			for _, m := range file.MessageType {
				testMsg(m, 0)
			}
		})

		Convey("synthesized message", func() {
			myFakeMessage := mkMessage(
				"myMessage",
				mkField("f1", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, nil),
				mkField("st", 2, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, &structpb.Struct{}),
			)
			printer.AppendLeadingComments(myFakeMessage, []string{"Message comment", "second line."})
			printer.AppendLeadingComments(myFakeMessage.Field[0], []string{"simple string"})
			printer.AppendLeadingComments(myFakeMessage.Field[1], []string{"cool message type"})

			printer.Message(myFakeMessage)
			So(buf.String(), ShouldEqual, `// Message comment
// second line.
message myMessage {
	// simple string
	string f1 = 1;
	// cool message type
	google.protobuf.Struct st = 2;
}
`)
		})
	})
}

func mkField(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type, msg proto.Message) *descriptorpb.FieldDescriptorProto {
	ret := &descriptorpb.FieldDescriptorProto{}
	ret.Name = &name
	camelName := camel(name)
	ret.JsonName = &camelName
	ret.Number = &num
	ret.Type = &typ
	if typ == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		fn := string(msg.ProtoReflect().Descriptor().FullName())
		ret.TypeName = &fn
	}
	return ret
}

func mkMessage(name string, fields ...*descriptorpb.FieldDescriptorProto) *descriptorpb.DescriptorProto {
	ret := &descriptorpb.DescriptorProto{}
	ret.Name = &name
	ret.Field = fields
	return ret
}
