// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proto

import (
	"crypto/sha256"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/proto/internal/testingpb"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestStableHash(t *testing.T) {
	Convey("stable hash for proto message", t, func() {
		h := sha256.New()
		Convey("simple proto", func() {
			err := StableHash(h, &testingpb.Simple{
				Id:   1,
				Some: &testingpb.Some{I: 2},
			})
			So(err, ShouldBeNil)
			So(fmt.Sprintf("%x", h.Sum(nil)), ShouldEqual, "09abb21ea7df951884db3c940c10866cc7949d4b93c3b498e39144e1e2e7a50e")
		})
		Convey("full proto", func() {
			err := StableHash(h, &testingpb.Full{
				Nums:       []int32{1, 2, 3},
				Strs:       []string{"a", "b"},
				Msg:        &testingpb.Full{I32: -1, I64: -2, U32: 1, U64: 2, F32: 1.0, F64: 2.0},
				MapStrNum:  map[string]int32{"x": 1, "y": 2},
				MapNumStr:  map[int32]string{1: "x", 2: "y"},
				MapBoolStr: map[bool]string{true: "x", false: "y"},
			})
			So(err, ShouldBeNil)
			So(fmt.Sprintf("%x", h.Sum(nil)), ShouldEqual, "a03f498e9953da367ad852a7cc3a5825c4b5d37b31108065efc90542ee14bcfd")
		})
		Convey("full proto - empty", func() {
			err := StableHash(h, &testingpb.Full{})
			So(err, ShouldBeNil)
			So(fmt.Sprintf("%x", h.Sum(nil)), ShouldEqual, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
		})
		Convey("any proto", func() {
			msg, err := anypb.New(&testingpb.Simple{})
			So(err, ShouldBeNil)
			err = StableHash(h, msg)
			So(err, ShouldBeNil)
		})
		Convey("any proto without type", func() {
			err := StableHash(h, &anypb.Any{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid empty type URL")
		})
		Convey("unknown field", func() {
			drv := &testingpb.Simple{}
			drv.ProtoReflect().SetUnknown([]byte{0})
			err := StableHash(h, drv)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unknown fields cannot be hashed")
		})
	})
}
