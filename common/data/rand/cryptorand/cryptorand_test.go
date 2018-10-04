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

package cryptorand

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCryptoRand(t *testing.T) {
	t.Parallel()

	Convey("test unmocked real rand", t, func() {
		// Read bytes from real rand, make sure we don't crash.
		n, err := Read(context.Background(), make([]byte, 16))
		So(n, ShouldEqual, 16)
		So(err, ShouldBeNil)
	})

	Convey("test mocked rand", t, func() {
		ctx := MockForTest(context.Background(), 0)
		buf := make([]byte, 16)
		n, err := Read(ctx, buf)
		So(n, ShouldEqual, 16)
		So(err, ShouldBeNil)
		So(buf, ShouldResemble, []byte{0xfa, 0x12, 0xf9, 0x2a, 0xfb, 0xe0, 0xf,
			0x85, 0x8, 0xd0, 0xe8, 0x3b, 0xab, 0x9c, 0xf8, 0xce})
	})
}
