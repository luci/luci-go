// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rawdatastore

import (
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func mkKey(aid, ns string, elems ...interface{}) Key {
	if len(elems)%2 != 0 {
		panic("odd number of tokens")
	}
	toks := make([]KeyTok, len(elems)/2)
	for i := 0; i < len(elems); i += 2 {
		toks[i/2].Kind = elems[i].(string)
		switch x := elems[i+1].(type) {
		case string:
			toks[i/2].StringID = x
		case int:
			toks[i/2].IntID = int64(x)
		default:
			panic("bad token id")
		}
	}
	return NewKeyToks(aid, ns, toks)
}

func ShouldEqualKey(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return fmt.Sprintf("Assertion requires 1 expected value, got %d", len(expected))
	}
	if KeysEqual(actual.(Key), expected[0].(Key)) {
		return ""
	}
	return fmt.Sprintf("Expected: %q\nActual: %q", actual, expected[0])
}

func TestKeyEncode(t *testing.T) {
	t.Parallel()

	keys := []Key{
		mkKey("appid", "ns", "kind", 1),
		mkKey("appid", "ns", "nerd", "moo"),
		mkKey("appid", "ns", "parent", 10, "renerd", "moo"),
	}

	Convey("Key Round trip", t, func() {
		for _, k := range keys {
			k := k
			Convey(k.String(), func() {
				enc := KeyEncode(k)
				aid, ns, toks, err := KeyToksDecode(enc)
				So(err, ShouldBeNil)
				dec := NewKeyToks(aid, ns, toks)
				So(dec, ShouldNotBeNil)
				So(dec, ShouldEqualKey, k)

				dec2, err := NewKeyFromEncoded(enc)
				So(err, ShouldBeNil)
				So(dec2, ShouldEqualKey, dec)
				So(dec2, ShouldEqualKey, k)
			})

			Convey(k.String()+" (json)", func() {
				data, err := KeyMarshalJSON(k)
				So(err, ShouldBeNil)

				aid, ns, toks, err := KeyUnmarshalJSON(data)
				So(err, ShouldBeNil)
				So(NewKeyToks(aid, ns, toks), ShouldEqualKey, k)
			})
		}
	})

	Convey("NewKey", t, func() {
		Convey("single", func() {
			k := NewKey("appid", "ns", "kind", "", 1, nil)
			So(k, ShouldEqualKey, keys[0])
		})

		Convey("nest", func() {
			k := NewKey("appid", "ns", "renerd", "moo", 0,
				NewKey("appid", "ns", "parent", "", 10, nil))
			So(k, ShouldEqualKey, keys[2])
		})
	})

	Convey("Key bad encoding", t, func() {
		Convey("extra junk before", func() {
			enc := KeyEncode(keys[2])
			_, _, _, err := KeyToksDecode("/" + enc)
			So(err, ShouldErrLike, "illegal base64")
		})

		Convey("extra junk after", func() {
			enc := KeyEncode(keys[2])
			_, _, _, err := KeyToksDecode(enc[:len(enc)-1])
			So(err, ShouldErrLike, "EOF")
		})

		Convey("json encoding includes quotes", func() {
			data, err := KeyMarshalJSON(keys[0])
			So(err, ShouldBeNil)

			_, _, _, err = KeyUnmarshalJSON(append(data, '!'))
			So(err, ShouldErrLike, "bad JSON key")
		})
	})
}

type dumbKey1 struct{ Key }

func (dk dumbKey1) Namespace() string { return "ns" }
func (dk dumbKey1) Parent() Key       { return dk.Key }
func (dk dumbKey1) String() string    { return "dumbKey1" }

type dumbKey2 struct{ Key }

/// This is the dumb part... can't have both IDs set.
func (dk dumbKey2) IntID() int64     { return 1 }
func (dk dumbKey2) StringID() string { return "wat" }

func (dk dumbKey2) Kind() string      { return "kind" }
func (dk dumbKey2) Parent() Key       { return nil }
func (dk dumbKey2) Namespace() string { return "ns" }
func (dk dumbKey2) AppID() string     { return "aid" }
func (dk dumbKey2) String() string    { return "dumbKey2" }

func TestBadKeyEncode(t *testing.T) {
	t.Parallel()

	Convey("bad keys", t, func() {
		Convey("incomplete", func() {
			So(KeyIncomplete(mkKey("aid", "ns", "kind", 1)), ShouldBeFalse)
			So(KeyIncomplete(mkKey("aid", "ns", "kind", 0)), ShouldBeTrue)
		})

		Convey("invalid", func() {
			So(KeyValid(mkKey("aid", "ns", "hat", "face", "__kind__", 1), "ns", true), ShouldBeTrue)
			So(KeyValid(mkKey("aid", "ns", "hat", "face", "kind", 1), "wat", false), ShouldBeFalse)

			bads := []Key{
				nil,
				mkKey("", "ns", "hat", "face"),
				mkKey("aid", "ns", "base", 1, "", "id"),
				mkKey("aid", "ns", "hat", "face", "__kind__", 1),
				mkKey("aid", "ns", "hat", 0, "kind", 1),
				dumbKey1{mkKey("aid", "badNS", "hat", 1)},
				dumbKey2{},
			}
			for _, k := range bads {
				s := "<nil>"
				if k != nil {
					s = k.String()
				}
				Convey(s, func() {
					So(KeyValid(k, "ns", false), ShouldBeFalse)
				})
			}
		})
	})
}

type keyWrap struct{ Key }

func (k keyWrap) Parent() Key {
	if k.Key.Parent() != nil {
		return keyWrap{k.Key.Parent()}
	}
	return nil
}

func TestMiscKey(t *testing.T) {
	t.Parallel()

	Convey("KeyRoot", t, func() {
		k := mkKey("appid", "ns", "parent", 10, "renerd", "moo")
		r := mkKey("appid", "ns", "parent", 10)
		So(KeyRoot(k), ShouldEqualKey, r)
		So(KeyRoot(nil), ShouldBeNil)
	})

	Convey("KeySplit", t, func() {
		// keyWrap forces KeySplit to not take the GenericKey shortcut.
		k := keyWrap{mkKey("appid", "ns", "parent", 10, "renerd", "moo")}
		aid, ns, toks := KeySplit(k)
		So(aid, ShouldEqual, "appid")
		So(ns, ShouldEqual, "ns")
		So(toks, ShouldResemble, []KeyTok{
			{Kind: "parent", IntID: 10},
			{Kind: "renerd", StringID: "moo"},
		})
	})

	Convey("KeySplit (nil)", t, func() {
		aid, ns, toks := KeySplit(nil)
		So(aid, ShouldEqual, "")
		So(ns, ShouldEqual, "")
		So(toks, ShouldResemble, []KeyTok(nil))
	})

	Convey("KeySplit ((*GenericKey)(nil))", t, func() {
		aid, ns, toks := KeySplit((*GenericKey)(nil))
		So(aid, ShouldEqual, "")
		So(ns, ShouldEqual, "")
		So(toks, ShouldResemble, []KeyTok(nil))
	})

	Convey("KeysEqual", t, func() {
		k1 := mkKey("a", "n", "knd", 1)
		k2 := mkKey("a", "n", "knd", 1)
		So(KeysEqual(k1, k2), ShouldBeTrue)
		k3 := mkKey("a", "n", "knd", 2)
		So(KeysEqual(k1, k3), ShouldBeFalse)
	})

	Convey("KeyString", t, func() {
		k1 := mkKey("a", "n", "knd", 1, "other", "wat")
		So(KeyString(k1), ShouldEqual, "/knd,1/other,wat")
		So(KeyString(nil), ShouldEqual, "")
	})

	Convey("*GenericKey supports json encoding", t, func() {
		type TestStruct struct {
			Key *GenericKey
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
		So(t, ShouldResemble, t2)
	})
}
