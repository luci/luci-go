// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package helper

import (
	"encoding/json"
	"fmt"
	"testing"

	"infra/gae/libs/gae"

	. "github.com/smartystreets/goconvey/convey"
)

func mkKey(aid, ns string, elems ...interface{}) gae.DSKey {
	if len(elems)%2 != 0 {
		panic("odd number of tokens")
	}
	toks := make([]gae.DSKeyTok, len(elems)/2)
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
	return NewDSKeyToks(aid, ns, toks)
}

func ShouldEqualKey(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return fmt.Sprintf("Assertion requires 1 expected value, got %d", len(expected))
	}
	if DSKeysEqual(actual.(gae.DSKey), expected[0].(gae.DSKey)) {
		return ""
	}
	return fmt.Sprintf("Expected: %q\nActual: %q", actual, expected[0])
}

func TestKeyEncode(t *testing.T) {
	t.Parallel()

	keys := []gae.DSKey{
		mkKey("appid", "ns", "kind", 1),
		mkKey("appid", "ns", "nerd", "moo"),
		mkKey("appid", "ns", "parent", 10, "renerd", "moo"),
	}

	Convey("DSKey Round trip", t, func() {
		for _, k := range keys {
			k := k
			Convey(k.String(), func() {
				enc := DSKeyEncode(k)
				aid, ns, toks, err := DSKeyToksDecode(enc)
				So(err, ShouldBeNil)
				dec := NewDSKeyToks(aid, ns, toks)
				So(dec, ShouldNotBeNil)
				So(dec, ShouldEqualKey, k)

				dec2, err := NewDSKeyFromEncoded(enc)
				So(err, ShouldBeNil)
				So(dec2, ShouldEqualKey, dec)
				So(dec2, ShouldEqualKey, k)
			})

			Convey(k.String()+" (json)", func() {
				data, err := DSKeyMarshalJSON(k)
				So(err, ShouldBeNil)

				aid, ns, toks, err := DSKeyUnmarshalJSON(data)
				So(err, ShouldBeNil)
				So(NewDSKeyToks(aid, ns, toks), ShouldEqualKey, k)
			})
		}
	})

	Convey("NewDSKey", t, func() {
		Convey("single", func() {
			k := NewDSKey("appid", "ns", "kind", "", 1, nil)
			So(k, ShouldEqualKey, keys[0])
		})

		Convey("nest", func() {
			k := NewDSKey("appid", "ns", "renerd", "moo", 0,
				NewDSKey("appid", "ns", "parent", "", 10, nil))
			So(k, ShouldEqualKey, keys[2])
		})
	})

	Convey("DSKey bad encoding", t, func() {
		Convey("extra junk before", func() {
			enc := DSKeyEncode(keys[2])
			_, _, _, err := DSKeyToksDecode("/" + enc)
			So(err, ShouldErrLike, "illegal base64")
		})

		Convey("extra junk after", func() {
			enc := DSKeyEncode(keys[2])
			_, _, _, err := DSKeyToksDecode(enc[:len(enc)-1])
			So(err, ShouldErrLike, "EOF")
		})

		Convey("json encoding includes quotes", func() {
			data, err := DSKeyMarshalJSON(keys[0])
			So(err, ShouldBeNil)

			_, _, _, err = DSKeyUnmarshalJSON(append(data, '!'))
			So(err, ShouldErrLike, "bad JSON key")
		})
	})
}

type dumbKey1 struct{ gae.DSKey }

func (dk dumbKey1) Namespace() string { return "ns" }
func (dk dumbKey1) Parent() gae.DSKey { return dk.DSKey }
func (dk dumbKey1) String() string    { return "dumbKey1" }

type dumbKey2 struct{ gae.DSKey }

/// This is the dumb part... can't have both IDs set.
func (dk dumbKey2) IntID() int64     { return 1 }
func (dk dumbKey2) StringID() string { return "wat" }

func (dk dumbKey2) Kind() string      { return "kind" }
func (dk dumbKey2) Parent() gae.DSKey { return nil }
func (dk dumbKey2) Namespace() string { return "ns" }
func (dk dumbKey2) AppID() string     { return "aid" }
func (dk dumbKey2) String() string    { return "dumbKey2" }

func TestBadKeyEncode(t *testing.T) {
	t.Parallel()

	Convey("bad keys", t, func() {
		Convey("incomplete", func() {
			So(DSKeyIncomplete(mkKey("aid", "ns", "kind", 1)), ShouldBeFalse)
			So(DSKeyIncomplete(mkKey("aid", "ns", "kind", 0)), ShouldBeTrue)
		})

		Convey("invalid", func() {
			So(DSKeyValid(mkKey("aid", "ns", "hat", "face", "__kind__", 1), "ns", true), ShouldBeTrue)
			So(DSKeyValid(mkKey("aid", "ns", "hat", "face", "kind", 1), "wat", false), ShouldBeFalse)

			bads := []gae.DSKey{
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
					So(DSKeyValid(k, "ns", false), ShouldBeFalse)
				})
			}
		})
	})
}

type keyWrap struct{ gae.DSKey }

func (k keyWrap) Parent() gae.DSKey {
	if k.DSKey.Parent() != nil {
		return keyWrap{k.DSKey.Parent()}
	}
	return nil
}

func TestMiscKey(t *testing.T) {
	t.Parallel()

	Convey("DSKeyRoot", t, func() {
		k := mkKey("appid", "ns", "parent", 10, "renerd", "moo")
		r := mkKey("appid", "ns", "parent", 10)
		So(DSKeyRoot(k), ShouldEqualKey, r)
		So(DSKeyRoot(nil), ShouldBeNil)
	})

	Convey("DSKeySplit", t, func() {
		// keyWrap forces DSKeySplit to not take the GenericDSKey shortcut.
		k := keyWrap{mkKey("appid", "ns", "parent", 10, "renerd", "moo")}
		aid, ns, toks := DSKeySplit(k)
		So(aid, ShouldEqual, "appid")
		So(ns, ShouldEqual, "ns")
		So(toks, ShouldResemble, []gae.DSKeyTok{
			{Kind: "parent", IntID: 10},
			{Kind: "renerd", StringID: "moo"},
		})
	})

	Convey("DSKeySplit (nil)", t, func() {
		aid, ns, toks := DSKeySplit(nil)
		So(aid, ShouldEqual, "")
		So(ns, ShouldEqual, "")
		So(toks, ShouldResemble, []gae.DSKeyTok(nil))
	})

	Convey("DSKeySplit ((*GenericDSKey)(nil))", t, func() {
		aid, ns, toks := DSKeySplit((*GenericDSKey)(nil))
		So(aid, ShouldEqual, "")
		So(ns, ShouldEqual, "")
		So(toks, ShouldResemble, []gae.DSKeyTok(nil))
	})

	Convey("DSKeysEqual", t, func() {
		k1 := mkKey("a", "n", "knd", 1)
		k2 := mkKey("a", "n", "knd", 1)
		So(DSKeysEqual(k1, k2), ShouldBeTrue)
		k3 := mkKey("a", "n", "knd", 2)
		So(DSKeysEqual(k1, k3), ShouldBeFalse)
	})

	Convey("DSKeyString", t, func() {
		k1 := mkKey("a", "n", "knd", 1, "other", "wat")
		So(DSKeyString(k1), ShouldEqual, "/knd,1/other,wat")
		So(DSKeyString(nil), ShouldEqual, "")
	})

	Convey("*GenericDSKey supports json encoding", t, func() {
		type TestStruct struct {
			Key *GenericDSKey
		}
		t := &TestStruct{
			NewDSKey("aid", "ns", "kind", "id", 0,
				NewDSKey("aid", "ns", "parent", "", 1, nil),
			)}
		d, err := json.Marshal(t)
		So(err, ShouldBeNil)
		t2 := &TestStruct{}
		err = json.Unmarshal(d, t2)
		So(err, ShouldBeNil)
		So(t, ShouldResemble, t2)
	})
}
