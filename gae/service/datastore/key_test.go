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

package datastore

import (
	"encoding/json"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func ShouldEqualKey(actual any, expected ...any) string {
	if len(expected) != 1 {
		return fmt.Sprintf("Assertion requires 1 expected value, got %d", len(expected))
	}
	if actual.(*Key).Equal(expected[0].(*Key)) {
		return ""
	}
	return fmt.Sprintf("Expected: %q\nActual: %q", actual, expected[0])
}

func TestKeyEncode(t *testing.T) {
	t.Parallel()

	kc := MkKeyContext("appid", "ns")
	keys := []*Key{
		kc.MakeKey("kind", 1),
		kc.MakeKey("nerd", "moo"),
		kc.MakeKey("parent", 10, "renerd", "moo"),
	}

	ftt.Run("Key Round trip", t, func(t *ftt.Test) {
		for _, k := range keys {
			t.Run(k.String(), func(t *ftt.Test) {
				enc := k.Encode()
				dec, err := NewKeyEncoded(enc)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, dec, should.NotBeNil)
				assert.Loosely(t, dec, convey.Adapt(ShouldEqualKey)(k))

				dec2, err := NewKeyEncoded(enc)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, dec2, convey.Adapt(ShouldEqualKey)(dec))
				assert.Loosely(t, dec2, convey.Adapt(ShouldEqualKey)(k))
			})

			t.Run(k.String()+" (json)", func(t *ftt.Test) {
				data, err := k.MarshalJSON()
				assert.Loosely(t, err, should.BeNil)

				dec := &Key{}
				assert.Loosely(t, dec.UnmarshalJSON(data), should.BeNil)
				assert.Loosely(t, dec, convey.Adapt(ShouldEqualKey)(k))
			})
		}
	})

	ftt.Run("NewKey", t, func(t *ftt.Test) {
		t.Run("single", func(t *ftt.Test) {
			k := MkKeyContext("appid", "ns").NewKey("kind", "", 1, nil)
			assert.Loosely(t, k, convey.Adapt(ShouldEqualKey)(keys[0]))
		})

		t.Run("empty", func(t *ftt.Test) {
			assert.Loosely(t, MkKeyContext("appid", "ns").NewKeyToks(nil), should.BeNil)
		})

		t.Run("nest", func(t *ftt.Test) {
			kc := MkKeyContext("appid", "ns")
			k := kc.NewKey("renerd", "moo", 0, kc.NewKey("parent", "", 10, nil))
			assert.Loosely(t, k, convey.Adapt(ShouldEqualKey)(keys[2]))
		})
	})

	ftt.Run("Key bad encoding", t, func(t *ftt.Test) {
		t.Run("extra junk before", func(t *ftt.Test) {
			enc := keys[2].Encode()
			_, err := NewKeyEncoded("/" + enc)
			assert.Loosely(t, err, should.ErrLike("illegal base64"))
		})

		t.Run("extra junk after", func(t *ftt.Test) {
			enc := keys[2].Encode()
			_, err := NewKeyEncoded(enc[:len(enc)-1])
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("json encoding includes quotes", func(t *ftt.Test) {
			data, err := keys[0].MarshalJSON()
			assert.Loosely(t, err, should.BeNil)

			dec := &Key{}
			err = dec.UnmarshalJSON(append(data, '!'))
			assert.Loosely(t, err, should.ErrLike("bad JSON key"))
		})
	})
}

func TestKeyValidity(t *testing.T) {
	t.Parallel()

	ftt.Run("keys validity", t, func(t *ftt.Test) {
		kc := MkKeyContext("aid", "ns")

		t.Run("incomplete", func(t *ftt.Test) {
			assert.Loosely(t, kc.MakeKey("kind", 1).IsIncomplete(), should.BeFalse)
			assert.Loosely(t, kc.MakeKey("kind", 0).IsIncomplete(), should.BeTrue)
		})

		t.Run("invalid", func(t *ftt.Test) {
			assert.Loosely(t, kc.MakeKey("hat", "face", "__kind__", 1).Valid(true, kc), should.BeTrue)

			bads := []*Key{
				MkKeyContext("aid", "ns").NewKeyToks([]KeyTok{{"Kind", 1, "1"}}),
				MkKeyContext("", "ns").MakeKey("", "ns", "hat", "face"),
				kc.MakeKey("base", 1, "", "id"),
				kc.MakeKey("hat", "face", "__kind__", 1),
				kc.MakeKey("hat", 0, "kind", 1),
			}
			for _, k := range bads {
				t.Run(k.String(), func(t *ftt.Test) {
					assert.Loosely(t, k.Valid(false, kc), should.BeFalse)
				})
			}
		})

		t.Run("partially valid", func(t *ftt.Test) {
			assert.Loosely(t, kc.MakeKey("kind", "").PartialValid(kc), should.BeTrue)
			assert.Loosely(t, kc.MakeKey("kind", "", "child", "").PartialValid(kc), should.BeFalse)
		})
	})
}

func TestMiscKey(t *testing.T) {
	t.Parallel()

	ftt.Run("KeyRoot", t, func(t *ftt.Test) {
		kc := MkKeyContext("appid", "ns")

		k := kc.MakeKey("parent", 10, "renerd", "moo")
		r := kc.MakeKey("parent", 10)
		assert.Loosely(t, k.Root(), convey.Adapt(ShouldEqualKey)(r))
	})

	ftt.Run("KeysEqual", t, func(t *ftt.Test) {
		kc := MkKeyContext("a", "n")

		k1 := kc.MakeKey("knd", 1)
		k2 := kc.MakeKey("knd", 1)
		assert.Loosely(t, k1.Equal(k2), should.BeTrue)
		k3 := kc.MakeKey("knd", 2)
		assert.Loosely(t, k1.Equal(k3), should.BeFalse)
	})

	ftt.Run("KeyString", t, func(t *ftt.Test) {
		kc := MkKeyContext("a", "n")

		k1 := kc.MakeKey("knd", 1, "other", "wat")
		assert.Loosely(t, k1.String(), should.Equal("a:n:/knd,1/other,\"wat\""))
	})

	ftt.Run("HasAncestor", t, func(t *ftt.Test) {
		kc := MkKeyContext("a", "n")

		k1 := kc.MakeKey("kind", 1)
		k2 := kc.MakeKey("kind", 1, "other", "wat")
		k3 := kc.MakeKey("kind", 1, "other", "wat", "extra", "data")
		k4 := MkKeyContext("something", "n").MakeKey("kind", 1)
		k5 := kc.MakeKey("kind", 1, "other", "meep")

		assert.Loosely(t, k1.HasAncestor(k1), should.BeTrue)
		assert.Loosely(t, k1.HasAncestor(k2), should.BeFalse)
		assert.Loosely(t, k2.HasAncestor(k5), should.BeFalse)
		assert.Loosely(t, k5.HasAncestor(k2), should.BeFalse)
		assert.Loosely(t, k2.HasAncestor(k1), should.BeTrue)
		assert.Loosely(t, k3.HasAncestor(k2), should.BeTrue)
		assert.Loosely(t, k3.HasAncestor(k1), should.BeTrue)
		assert.Loosely(t, k3.HasAncestor(k4), should.BeFalse)
	})

	ftt.Run("*GenericKey supports json encoding", t, func(t *ftt.Test) {
		type TestStruct struct {
			Key *Key
		}
		ts := &TestStruct{
			MkKeyContext("aid", "ns").NewKey("kind", "id", 0,
				MkKeyContext("aid", "ns").NewKey("parent", "", 1, nil),
			)}
		d, err := json.Marshal(ts)
		assert.Loosely(t, err, should.BeNil)
		t2 := &TestStruct{}
		err = json.Unmarshal(d, t2)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ts.Key, convey.Adapt(ShouldEqualKey)(t2.Key))
	})
}

func shouldBeLess(actual any, expected ...any) string {
	a, b := actual.(*Key), expected[0].(*Key)
	if a.Less(b) {
		return ""
	}
	return fmt.Sprintf("Expected %q < %q", a.String(), b.String())
}

func shouldNotBeEqual(actual any, expected ...any) string {
	a, b := actual.(*Key), expected[0].(*Key)
	if !a.Equal(b) {
		return ""
	}
	return fmt.Sprintf("Expected %q != %q", a.String(), b.String())
}

func shouldNotBeLess(actual any, expected ...any) string {
	a, b := actual.(*Key), expected[0].(*Key)
	if !a.Less(b) {
		return ""
	}
	return fmt.Sprintf("Expected !(%q < %q)", a.String(), b.String())
}

func TestKeySort(t *testing.T) {
	t.Parallel()

	ftt.Run("KeyTok.Less() works", t, func(t *ftt.Test) {
		assert.Loosely(t, (KeyTok{"a", 0, "1"}).Less(KeyTok{"b", 0, "2"}), should.BeTrue)
		assert.Loosely(t, (KeyTok{"b", 0, "1"}).Less(KeyTok{"a", 0, "2"}), should.BeFalse)
		assert.Loosely(t, (KeyTok{"kind", 0, "1"}).Less(KeyTok{"kind", 0, "2"}), should.BeTrue)
		assert.Loosely(t, (KeyTok{"kind", 1, ""}).Less(KeyTok{"kind", 2, ""}), should.BeTrue)
		assert.Loosely(t, (KeyTok{"kind", 1, ""}).Less(KeyTok{"kind", 0, "1"}), should.BeTrue)
	})

	ftt.Run("Key comparison works", t, func(t *ftt.Test) {
		s := []*Key{
			MkKeyContext("A", "").MakeKey("kind", 1),
			MkKeyContext("A", "n").MakeKey("kind", 1),
			MkKeyContext("A", "n").MakeKey("kind", 1, "something", "else"),
			MkKeyContext("A", "n").MakeKey("kind", "1"),
			MkKeyContext("A", "n").MakeKey("kind", "1", "something", "else"),
			MkKeyContext("A", "n").MakeKey("other", 1, "something", "else"),
			MkKeyContext("a", "").MakeKey("kind", 1),
			MkKeyContext("a", "n").MakeKey("kind", 1),
			MkKeyContext("a", "n").MakeKey("kind", 2),
			MkKeyContext("a", "p").MakeKey("aleph", 1),
			MkKeyContext("b", "n").MakeKey("kind", 2),
		}

		for i := 1; i < len(s); i++ {
			assert.Loosely(t, s[i-1], convey.Adapt(shouldBeLess)(s[i]))
			assert.Loosely(t, s[i-1], convey.Adapt(shouldNotBeEqual)(s[i]))
			assert.Loosely(t, s[i], convey.Adapt(shouldNotBeEqual)(s[i-1]))
			assert.Loosely(t, s[i], convey.Adapt(shouldNotBeLess)(s[i-1]))
		}
	})
}
