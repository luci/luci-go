// Copyright 2019 The LUCI Authors.
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
	"reflect"

	"github.com/golang/protobuf/proto"

	luciproto "go.chromium.org/luci/common/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func textpb(pb proto.Message, textML string) proto.Message {
	if err := luciproto.UnmarshalTextML(textML, pb); err != nil {
		panic(err)
	}
	return pb
}

func shouldResembleProtoTextML(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return fmt.Sprintf("shouldResembleProtoTextML expects 1 value, got %d", len(expected))
	}

	expectedMsg := reflect.New(reflect.TypeOf(actual).Elem()).Interface().(proto.Message)
	err := luciproto.UnmarshalTextML(expected[0].(string), expectedMsg)
	So(err, ShouldBeNil)
	So(actual, ShouldResembleProto, expectedMsg)
	return ""
}
