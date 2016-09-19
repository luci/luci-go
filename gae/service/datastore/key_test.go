// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

	kc := KeyContext{"appid", "ns"}
	keys := []*Key{
		kc.MakeKey("kind", 1),
		kc.MakeKey("nerd", "moo"),
		kc.MakeKey("parent", 10, "renerd", "moo"),
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
			k := KeyContext{"appid", "ns"}.NewKey("kind", "", 1, nil)
			So(k, ShouldEqualKey, keys[0])
		})

		Convey("empty", func() {
			So(KeyContext{"appid", "ns"}.NewKeyToks(nil), ShouldBeNil)
		})

		Convey("nest", func() {
			kc := KeyContext{"appid", "ns"}
			k := kc.NewKey("renerd", "moo", 0, kc.NewKey("parent", "", 10, nil))
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
	t.Parallel()

	Convey("keys validity", t, func() {
		kc := KeyContext{"aid", "ns"}

		Convey("incomplete", func() {
			So(kc.MakeKey("kind", 1).IsIncomplete(), ShouldBeFalse)
			So(kc.MakeKey("kind", 0).IsIncomplete(), ShouldBeTrue)
		})

		Convey("invalid", func() {
			So(kc.MakeKey("hat", "face", "__kind__", 1).Valid(true, kc), ShouldBeTrue)

			bads := []*Key{
				KeyContext{"aid", "ns"}.NewKeyToks([]KeyTok{{"Kind", 1, "1"}}),
				KeyContext{"", "ns"}.MakeKey("", "ns", "hat", "face"),
				kc.MakeKey("base", 1, "", "id"),
				kc.MakeKey("hat", "face", "__kind__", 1),
				kc.MakeKey("hat", 0, "kind", 1),
			}
			for _, k := range bads {
				Convey(k.String(), func() {
					So(k.Valid(false, kc), ShouldBeFalse)
				})
			}
		})

		Convey("partially valid", func() {
			So(kc.MakeKey("kind", "").PartialValid(kc), ShouldBeTrue)
			So(kc.MakeKey("kind", "", "child", "").PartialValid(kc), ShouldBeFalse)
		})
	})
}

func TestMiscKey(t *testing.T) {
	t.Parallel()

	Convey("KeyRoot", t, func() {
		kc := KeyContext{"appid", "ns"}

		k := kc.MakeKey("parent", 10, "renerd", "moo")
		r := kc.MakeKey("parent", 10)
		So(k.Root(), ShouldEqualKey, r)
	})

	Convey("KeysEqual", t, func() {
		kc := KeyContext{"a", "n"}

		k1 := kc.MakeKey("knd", 1)
		k2 := kc.MakeKey("knd", 1)
		So(k1.Equal(k2), ShouldBeTrue)
		k3 := kc.MakeKey("knd", 2)
		So(k1.Equal(k3), ShouldBeFalse)
	})

	Convey("KeyString", t, func() {
		kc := KeyContext{"a", "n"}

		k1 := kc.MakeKey("knd", 1, "other", "wat")
		So(k1.String(), ShouldEqual, "a:n:/knd,1/other,\"wat\"")
	})

	Convey("HasAncestor", t, func() {
		kc := KeyContext{"a", "n"}

		k1 := kc.MakeKey("kind", 1)
		k2 := kc.MakeKey("kind", 1, "other", "wat")
		k3 := kc.MakeKey("kind", 1, "other", "wat", "extra", "data")
		k4 := KeyContext{"something", "n"}.MakeKey("kind", 1)
		k5 := kc.MakeKey("kind", 1, "other", "meep")

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
			KeyContext{"aid", "ns"}.NewKey("kind", "id", 0,
				KeyContext{"aid", "ns"}.NewKey("parent", "", 1, nil),
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
			KeyContext{"A", ""}.MakeKey("kind", 1),
			KeyContext{"A", "n"}.MakeKey("kind", 1),
			KeyContext{"A", "n"}.MakeKey("kind", 1, "something", "else"),
			KeyContext{"A", "n"}.MakeKey("kind", "1"),
			KeyContext{"A", "n"}.MakeKey("kind", "1", "something", "else"),
			KeyContext{"A", "n"}.MakeKey("other", 1, "something", "else"),
			KeyContext{"a", ""}.MakeKey("kind", 1),
			KeyContext{"a", "n"}.MakeKey("kind", 1),
			KeyContext{"a", "n"}.MakeKey("kind", 2),
			KeyContext{"a", "p"}.MakeKey("aleph", 1),
			KeyContext{"b", "n"}.MakeKey("kind", 2),
		}

		for i := 1; i < len(s); i++ {
			So(s[i-1], shouldBeLess, s[i])
			So(s[i-1], shouldNotBeEqual, s[i])
			So(s[i], shouldNotBeEqual, s[i-1])
			So(s[i], shouldNotBeLess, s[i-1])
		}
	})
}
