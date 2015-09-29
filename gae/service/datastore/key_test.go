// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func ShouldEqualKey(actual interface{}, expected ...interface{}) string {
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

	keys := []*Key{
		MakeKey("appid", "ns", "kind", 1),
		MakeKey("appid", "ns", "nerd", "moo"),
		MakeKey("appid", "ns", "parent", 10, "renerd", "moo"),
	}

	Convey("Key Round trip", t, func() {
		for _, k := range keys {
			k := k
			Convey(k.String(), func() {
				enc := k.Encode()
				dec, err := NewKeyEncoded(enc)
				So(err, ShouldBeNil)
				So(dec, ShouldNotBeNil)
				So(dec, ShouldEqualKey, k)

				dec2, err := NewKeyEncoded(enc)
				So(err, ShouldBeNil)
				So(dec2, ShouldEqualKey, dec)
				So(dec2, ShouldEqualKey, k)
			})

			Convey(k.String()+" (json)", func() {
				data, err := k.MarshalJSON()
				So(err, ShouldBeNil)

				dec := &Key{}
				So(dec.UnmarshalJSON(data), ShouldBeNil)
				So(dec, ShouldEqualKey, k)
			})
		}
	})

	Convey("NewKey", t, func() {
		Convey("single", func() {
			k := NewKey("appid", "ns", "kind", "", 1, nil)
			So(k, ShouldEqualKey, keys[0])
		})

		Convey("empty", func() {
			So(NewKeyToks("appid", "ns", nil), ShouldBeNil)
		})

		Convey("nest", func() {
			k := NewKey("appid", "ns", "renerd", "moo", 0,
				NewKey("appid", "ns", "parent", "", 10, nil))
			So(k, ShouldEqualKey, keys[2])
		})
	})

	Convey("Key bad encoding", t, func() {
		Convey("extra junk before", func() {
			enc := keys[2].Encode()
			_, err := NewKeyEncoded("/" + enc)
			So(err, ShouldErrLike, "illegal base64")
		})

		Convey("extra junk after", func() {
			enc := keys[2].Encode()
			_, err := NewKeyEncoded(enc[:len(enc)-1])
			So(err, ShouldErrLike, "EOF")
		})

		Convey("json encoding includes quotes", func() {
			data, err := keys[0].MarshalJSON()
			So(err, ShouldBeNil)

			dec := &Key{}
			err = dec.UnmarshalJSON(append(data, '!'))
			So(err, ShouldErrLike, "bad JSON key")
		})
	})
}

func TestKeyValidity(t *testing.T) {
	//t.Parallel()

	Convey("keys validity", t, func() {
		Convey("incomplete", func() {
			So(MakeKey("aid", "ns", "kind", 1).Incomplete(), ShouldBeFalse)
			So(MakeKey("aid", "ns", "kind", 0).Incomplete(), ShouldBeTrue)
		})

		Convey("invalid", func() {
			So(MakeKey("aid", "ns", "hat", "face", "__kind__", 1).Valid(true, "aid", "ns"), ShouldBeTrue)

			bads := []*Key{
				NewKeyToks("aid", "ns", []KeyTok{{"Kind", 1, "1"}}),
				MakeKey("", "ns", "hat", "face"),
				MakeKey("aid", "ns", "base", 1, "", "id"),
				MakeKey("aid", "ns", "hat", "face", "__kind__", 1),
				MakeKey("aid", "ns", "hat", 0, "kind", 1),
			}
			for _, k := range bads {
				Convey(k.String(), func() {
					So(k.Valid(false, "aid", "ns"), ShouldBeFalse)
				})
			}
		})

		Convey("partially valid", func() {
			So(MakeKey("aid", "ns", "kind", "").PartialValid("aid", "ns"), ShouldBeTrue)
			So(MakeKey("aid", "ns", "kind", "", "child", "").PartialValid("aid", "ns"), ShouldBeFalse)
		})
	})
}

func TestMiscKey(t *testing.T) {
	t.Parallel()

	Convey("KeyRoot", t, func() {
		k := MakeKey("appid", "ns", "parent", 10, "renerd", "moo")
		r := MakeKey("appid", "ns", "parent", 10)
		So(k.Root(), ShouldEqualKey, r)
	})

	Convey("KeysEqual", t, func() {
		k1 := MakeKey("a", "n", "knd", 1)
		k2 := MakeKey("a", "n", "knd", 1)
		So(k1.Equal(k2), ShouldBeTrue)
		k3 := MakeKey("a", "n", "knd", 2)
		So(k1.Equal(k3), ShouldBeFalse)
	})

	Convey("KeyString", t, func() {
		k1 := MakeKey("a", "n", "knd", 1, "other", "wat")
		So(k1.String(), ShouldEqual, "a:n:/knd,1/other,\"wat\"")
	})

	Convey("HasAncestor", t, func() {
		k1 := MakeKey("a", "n", "kind", 1)
		k2 := MakeKey("a", "n", "kind", 1, "other", "wat")
		k3 := MakeKey("a", "n", "kind", 1, "other", "wat", "extra", "data")
		k4 := MakeKey("something", "n", "kind", 1)
		k5 := MakeKey("a", "n", "kind", 1, "other", "meep")

		So(k1.HasAncestor(k1), ShouldBeTrue)
		So(k1.HasAncestor(k2), ShouldBeFalse)
		So(k2.HasAncestor(k5), ShouldBeFalse)
		So(k5.HasAncestor(k2), ShouldBeFalse)
		So(k2.HasAncestor(k1), ShouldBeTrue)
		So(k3.HasAncestor(k2), ShouldBeTrue)
		So(k3.HasAncestor(k1), ShouldBeTrue)
		So(k3.HasAncestor(k4), ShouldBeFalse)
	})

	Convey("*GenericKey supports json encoding", t, func() {
		type TestStruct struct {
			Key *Key
		}
		t := &TestStruct{
			NewKey("aid", "ns", "kind", "id", 0,
				NewKey("aid", "ns", "parent", "", 1, nil),
			)}
		d, err := json.Marshal(t)
		So(err, ShouldBeNil)
		t2 := &TestStruct{}
		err = json.Unmarshal(d, t2)
		So(err, ShouldBeNil)
		So(t.Key, ShouldEqualKey, t2.Key)
	})
}

func shouldBeLess(actual interface{}, expected ...interface{}) string {
	a, b := actual.(*Key), expected[0].(*Key)
	if a.Less(b) {
		return ""
	}
	return fmt.Sprintf("Expected %q < %q", a.String(), b.String())
}

func shouldNotBeEqual(actual interface{}, expected ...interface{}) string {
	a, b := actual.(*Key), expected[0].(*Key)
	if !a.Equal(b) {
		return ""
	}
	return fmt.Sprintf("Expected %q != %q", a.String(), b.String())
}

func shouldNotBeLess(actual interface{}, expected ...interface{}) string {
	a, b := actual.(*Key), expected[0].(*Key)
	if !a.Less(b) {
		return ""
	}
	return fmt.Sprintf("Expected !(%q < %q)", a.String(), b.String())
}

func TestKeySort(t *testing.T) {
	t.Parallel()

	Convey("KeyTok.Less() works", t, func() {
		So((KeyTok{"a", 0, "1"}).Less(KeyTok{"b", 0, "2"}), ShouldBeTrue)
		So((KeyTok{"b", 0, "1"}).Less(KeyTok{"a", 0, "2"}), ShouldBeFalse)
		So((KeyTok{"kind", 0, "1"}).Less(KeyTok{"kind", 0, "2"}), ShouldBeTrue)
		So((KeyTok{"kind", 1, ""}).Less(KeyTok{"kind", 2, ""}), ShouldBeTrue)
		So((KeyTok{"kind", 1, ""}).Less(KeyTok{"kind", 0, "1"}), ShouldBeTrue)
	})

	Convey("Key comparison works", t, func() {
		s := []*Key{
			MakeKey("A", "", "kind", 1),
			MakeKey("A", "n", "kind", 1),
			MakeKey("A", "n", "kind", 1, "something", "else"),
			MakeKey("A", "n", "kind", "1"),
			MakeKey("A", "n", "kind", "1", "something", "else"),
			MakeKey("A", "n", "other", 1, "something", "else"),
			MakeKey("a", "", "kind", 1),
			MakeKey("a", "n", "kind", 1),
			MakeKey("a", "n", "kind", 2),
			MakeKey("a", "p", "aleph", 1),
			MakeKey("b", "n", "kind", 2),
		}

		for i := 1; i < len(s); i++ {
			So(s[i-1], shouldBeLess, s[i])
			So(s[i-1], shouldNotBeEqual, s[i])
			So(s[i], shouldNotBeEqual, s[i-1])
			So(s[i], shouldNotBeLess, s[i-1])
		}
	})
}
