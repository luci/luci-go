// Copyright 2015 The LUCI Authors.
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

package secrets

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBlobs(t *testing.T) {
	Convey("Blobs works", t, func() {
		s := Secret{
			Current: []byte("s1"),
			Previous: [][]byte{
				[]byte("s2"),
				[]byte("s3"),
			},
		}
		So(s.Blobs(), ShouldResemble, [][]byte{
			[]byte("s1"),
			[]byte("s2"),
			[]byte("s3"),
		})
	})
}

func TestContext(t *testing.T) {
	Convey("Works", t, func() {
		c := Set(context.Background(), StaticStore{
			"key": Secret{Current: []byte("secret")},
		})
		s, err := GetSecret(c, "key")
		So(err, ShouldBeNil)
		So(s.Current, ShouldResemble, []byte("secret"))

		_, err = GetSecret(c, "missing")
		So(err, ShouldEqual, ErrNoSuchSecret)

		// For code coverage.
		c = Set(c, nil)
		So(Get(c), ShouldBeNil)
		_, err = GetSecret(c, "key")
		So(err, ShouldEqual, ErrNoStoreConfigured)
	})
}
