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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSecret(t *testing.T) {
	t.Parallel()

	Convey("Blobs works", t, func() {
		s := Secret{
			Active: []byte("s1"),
			Passive: [][]byte{
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

	Convey("Equal works", t, func() {
		s1 := Secret{
			Active: []byte("1"),
			Passive: [][]byte{
				[]byte("2"),
				[]byte("3"),
			},
		}
		s2 := Secret{
			Active: []byte("1"),
			Passive: [][]byte{
				[]byte("2"),
			},
		}
		s3 := Secret{
			Active: []byte("zzz"),
			Passive: [][]byte{
				[]byte("2"),
				[]byte("3"),
			},
		}
		s4 := Secret{
			Active: []byte("1"),
			Passive: [][]byte{
				[]byte("2"),
				[]byte("zzz"),
			},
		}
		So(s1.Equal(s1), ShouldBeTrue)
		So(s1.Equal(s2), ShouldBeFalse)
		So(s1.Equal(s3), ShouldBeFalse)
		So(s1.Equal(s4), ShouldBeFalse)
	})
}
