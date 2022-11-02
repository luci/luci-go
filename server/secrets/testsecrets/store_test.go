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

package testsecrets

import (
	"context"
	"testing"

	"go.chromium.org/luci/server/secrets"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStore(t *testing.T) {
	ctx := context.Background()

	Convey("RandomSecret", t, func() {
		store := Store{}

		// Autogenerate one.
		s1, err := store.RandomSecret(ctx, "key1")
		So(err, ShouldBeNil)
		So(s1, ShouldResemble, secrets.Secret{
			Active: []byte{0xfa, 0x12, 0xf9, 0x2a, 0xfb, 0xe0, 0xf, 0x85},
		})

		// Getting same one back.
		s2, err := store.RandomSecret(ctx, "key1")
		So(err, ShouldBeNil)
		So(s2, ShouldResemble, secrets.Secret{
			Active: []byte{0xfa, 0x12, 0xf9, 0x2a, 0xfb, 0xe0, 0xf, 0x85},
		})
	})

	Convey("StoredSecret", t, func() {
		store := Store{
			Secrets: map[string]secrets.Secret{
				"key1": {Active: []byte("blah")},
			},
		}

		s, err := store.StoredSecret(ctx, "key1")
		So(err, ShouldBeNil)
		So(s, ShouldResemble, secrets.Secret{Active: []byte("blah")})

		_, err = store.StoredSecret(ctx, "key2")
		So(err, ShouldEqual, secrets.ErrNoSuchSecret)
	})
}
