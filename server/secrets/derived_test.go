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

package secrets

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDerivedStore(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		root := Secret{
			Current: []byte("1"),
			Previous: [][]byte{
				[]byte("2"),
				[]byte("3"),
			},
		}
		store := NewDerivedStore(root)

		s1, _ := store.GetSecret("secret_1")
		So(s1, ShouldResemble, Secret{
			Current: derive([]byte("1"), "secret_1"),
			Previous: [][]byte{
				derive([]byte("2"), "secret_1"),
				derive([]byte("3"), "secret_1"),
			},
		})

		// Test cache hit.
		s2, _ := store.GetSecret("secret_1")
		So(s2, ShouldResemble, s1)

		// Test noop root key change.
		store.SetRoot(root)
		s2, _ = store.GetSecret("secret_1")
		So(s2, ShouldResemble, s1)

		// Test actual root key change.
		store.SetRoot(Secret{Current: []byte("zzz")})
		s2, _ = store.GetSecret("secret_1")
		So(s2, ShouldNotResemble, s1)
	})
}

func benchmarkStore(b *testing.B, s Store) {
	names := []string{
		"secret_1",
		"secret_2",
		"secret_3",
	}
	for i := 0; i < b.N; i++ {
		s.GetSecret(names[i%3])
	}
}

func BenchmarkStaticStore(b *testing.B) {
	benchmarkStore(b, StaticStore{
		"secret_1": Secret{Current: []byte("val1")},
		"secret_2": Secret{Current: []byte("val2")},
		"secret_3": Secret{Current: []byte("val3")},
	})
}

func BenchmarkDerivedStore(b *testing.B) {
	benchmarkStore(b, NewDerivedStore(Secret{Current: []byte("root secret")}))
}
