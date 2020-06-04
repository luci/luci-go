// Copyright 2020 The LUCI Authors.
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

package flag

import (
	"flag"
	"fmt"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

// special type that implements proto.Message interface for test purpose
type messageImpl string

func (messageImpl) Reset() {}
func (m messageImpl) String() string {
	return string(m)
}
func (messageImpl) ProtoMessage() {}

// Assert that messageImpl implements to the proto.Message interface.
var _ = proto.Message(new(messageImpl))

func TestProtoMessageSliceFlag(t *testing.T) {
	Convey("Basic", t, func() {
		var builds []*pb.Build
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		flag := MessageSliceFlag(&builds)
		fs.Var(flag, "build", "Test Proto Message")

		var err error
		err = flag.Set("{\"id\": 111111, \"status\": \"SUCCESS\"}")
		So(err, ShouldBeNil)
		err = flag.Set("{\"id\": 222222, \"status\": \"FAILURE\"}")
		So(err, ShouldBeNil)
		err = flag.Set("{\"id\": 333333, \"status\": \"CANCELED\"}")
		So(err, ShouldBeNil)

		So(flag.Get(), ShouldResemble, builds)
		So(builds, ShouldHaveLength, 3)
		So(builds, ShouldContain,
			&pb.Build{Id: 111111, Status: pb.Status_SUCCESS})
		So(builds, ShouldContain,
			&pb.Build{Id: 222222, Status: pb.Status_FAILURE})
		So(builds, ShouldContain,
			&pb.Build{Id: 333333, Status: pb.Status_CANCELED})
		// Hard to set expectation for JSON string due to indentation
		// and field order. Therefore, simply check if it's empty or not.
		So(flag.String(), ShouldNotBeBlank)
	})
	Convey("Panic when input is not pointer", t, func() {
		So(func() { MessageSliceFlag("a string") }, ShouldPanic)
	})
	Convey("Panic when input pointer dose not point to a slice", t, func() {
		So(func() { MessageSliceFlag(&map[string]string{}) }, ShouldPanic)
	})
	Convey("Panic when element does not implement proto.Message", t, func() {
		So(func() { MessageSliceFlag(&[]string{}) }, ShouldPanic)
	})
	Convey("Panic when element is not pointer", t, func() {
		So(func() { MessageSliceFlag(&[]messageImpl{}) }, ShouldPanic)
	})
	Convey("Panic when element pointer does not point to a struct", t, func() {
		So(func() { MessageSliceFlag(&[]*messageImpl{}) }, ShouldPanic)
	})
}

func ExampleMessageSliceFlag() {
	var builds []*pb.Build
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	flag := MessageSliceFlag(&builds)
	fs.Var(flag, "build", "Test Proto Message")
	fs.Parse([]string{"-build",
		"{\"id\": 111111, \"status\": \"SUCCESS\"}"})
	marshaler := &jsonpb.Marshaler{Indent: "  "}
	jsonMsg, _ := marshaler.MarshalToString(builds[0])
	fmt.Println(jsonMsg)
	// Output:
	// {
	//   "id": "111111",
	//   "status": "SUCCESS"
	// }
}
