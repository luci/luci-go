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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	ftt.Run("Basic", t, func(t *ftt.Test) {
		var builds []*pb.Build
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		flag := MessageSliceFlag(&builds)
		fs.Var(flag, "build", "Test Proto Message")

		var err error
		err = flag.Set("{\"id\": 111111, \"status\": \"SUCCESS\"}")
		assert.Loosely(t, err, should.BeNil)
		err = flag.Set("{\"id\": 222222, \"status\": \"FAILURE\"}")
		assert.Loosely(t, err, should.BeNil)
		err = flag.Set("{\"id\": 333333, \"status\": \"CANCELED\"}")
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, flag.Get(), should.Resemble(builds))
		assert.Loosely(t, builds, should.HaveLength(3))
		assert.Loosely(t, builds[0].Id, should.Equal(111111))
		assert.Loosely(t, builds[0].Status, should.Equal(pb.Status_SUCCESS))
		assert.Loosely(t, builds[1].Id, should.Equal(222222))
		assert.Loosely(t, builds[1].Status, should.Equal(pb.Status_FAILURE))
		assert.Loosely(t, builds[2].Id, should.Equal(333333))
		assert.Loosely(t, builds[2].Status, should.Equal(pb.Status_CANCELED))
		// Hard to set expectation for JSON string due to indentation
		// and field order. Therefore, simply check if it's empty or not.
		assert.Loosely(t, flag.String(), should.NotBeBlank)
	})
	ftt.Run("Panic when input is not pointer", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { MessageSliceFlag("a string") }, should.Panic)
	})
	ftt.Run("Panic when input pointer dose not point to a slice", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { MessageSliceFlag(&map[string]string{}) }, should.Panic)
	})
	ftt.Run("Panic when element does not implement proto.Message", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { MessageSliceFlag(&[]string{}) }, should.Panic)
	})
	ftt.Run("Panic when element is not pointer", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { MessageSliceFlag(&[]messageImpl{}) }, should.Panic)
	})
	ftt.Run("Panic when element pointer does not point to a struct", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { MessageSliceFlag(&[]*messageImpl{}) }, should.Panic)
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
