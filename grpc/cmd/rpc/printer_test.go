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
	"io/ioutil"
	"strings"
	"testing"

	"go.chromium.org/luci/common/proto/google/descutil"

	"github.com/golang/protobuf/proto"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPrinter(t *testing.T) {
	t.Parallel()

	Convey("Printer", t, func() {
		protoFile, err := ioutil.ReadFile("printer_test.proto")
		So(err, ShouldBeNil)
		protoFileLines := strings.Split(string(protoFile), "\n")

		descFileBytes, err := ioutil.ReadFile("printer_test.desc")
		So(err, ShouldBeNil)

		var desc descriptor.FileDescriptorSet
		err = proto.Unmarshal(descFileBytes, &desc)
		So(err, ShouldBeNil)

		So(desc.File, ShouldHaveLength, 1)
		file := desc.File[0]

		sourceCodeInfo, err := descutil.IndexSourceCodeInfo(file)
		So(err, ShouldBeNil)

		getExpectedDef := func(ptr interface{}, unindent int) string {
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
		printer := newPrinter(&buf)
		So(printer.SetFile(file), ShouldBeNil)

		checkOutput := func(ptr interface{}, unindent int) {
			So(buf.String(), ShouldEqual, getExpectedDef(ptr, unindent))
		}

		Convey("package", func() {
			printer.Package(file.GetPackage())
			checkOutput(&file.Package, 0)
		})

		Convey("service", func() {
			for _, s := range file.Service {
				Convey(s.GetName(), func() {
					printer.Service(s, -1)
					checkOutput(s, 0)
				})
			}
		})

		testEnum := func(e *descriptor.EnumDescriptorProto, unindent int) {
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
			var testMsg func(*descriptor.DescriptorProto, int)
			testMsg = func(m *descriptor.DescriptorProto, unindent int) {
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
	})
}
