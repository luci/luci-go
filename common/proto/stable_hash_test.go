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

	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/common/proto/internal/testingpb"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStableHash(t *testing.T) {
	ftt.Run("stable hash for proto message", t, func(t *ftt.Test) {
		h := sha256.New()
		t.Run("simple proto", func(t *ftt.Test) {
			err := StableHash(h, &testingpb.Simple{
				Id:   1,
				Some: &testingpb.Some{I: 2},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fmt.Sprintf("%x", h.Sum(nil)), should.Equal("09abb21ea7df951884db3c940c10866cc7949d4b93c3b498e39144e1e2e7a50e"))
		})
		t.Run("full proto", func(t *ftt.Test) {
			err := StableHash(h, &testingpb.Full{
				Nums:       []int32{1, 2, 3},
				Strs:       []string{"a", "b"},
				Msg:        &testingpb.Full{I32: -1, I64: -2, U32: 1, U64: 2, F32: 1.0, F64: 2.0},
				MapStrNum:  map[string]int32{"x": 1, "y": 2},
				MapNumStr:  map[int32]string{1: "x", 2: "y"},
				MapBoolStr: map[bool]string{true: "x", false: "y"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fmt.Sprintf("%x", h.Sum(nil)), should.Equal("a03f498e9953da367ad852a7cc3a5825c4b5d37b31108065efc90542ee14bcfd"))
		})
		t.Run("full proto - empty", func(t *ftt.Test) {
			err := StableHash(h, &testingpb.Full{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fmt.Sprintf("%x", h.Sum(nil)), should.Equal("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"))
		})
		t.Run("any proto", func(t *ftt.Test) {
			msg, err := anypb.New(&testingpb.Simple{})
			assert.Loosely(t, err, should.BeNil)
			err = StableHash(h, msg)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("any proto without type", func(t *ftt.Test) {
			err := StableHash(h, &anypb.Any{})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid empty type URL"))
		})
		t.Run("unknown field", func(t *ftt.Test) {
			drv := &testingpb.Simple{}
			drv.ProtoReflect().SetUnknown([]byte{0})
			err := StableHash(h, drv)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("unknown fields cannot be hashed"))
		})
	})
}
