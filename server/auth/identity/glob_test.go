// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package identity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGlob(t *testing.T) {
	Convey("MakeGlob works", t, func() {
		g, err := MakeGlob("user:*@example.com")
		So(err, ShouldBeNil)
		So(g, ShouldEqual, Glob("user:*@example.com"))
		So(g.Kind(), ShouldEqual, User)
		So(g.Pattern(), ShouldEqual, "*@example.com")

		_, err = MakeGlob("bad ident")
		So(err, ShouldNotBeNil)
	})

	Convey("Validate works", t, func() {
		So(Glob("user:*@example.com").Validate(), ShouldBeNil)
		So(Glob("user:").Validate(), ShouldNotBeNil)
		So(Glob(":abc").Validate(), ShouldNotBeNil)
		So(Glob("abc@example.com").Validate(), ShouldNotBeNil)
		So(Glob("user:\n").Validate(), ShouldNotBeNil)
	})

	Convey("Kind works", t, func() {
		So(Glob("user:*@example.com").Kind(), ShouldEqual, User)
		So(Glob("???").Kind(), ShouldEqual, Anonymous)
	})

	Convey("Pattern works", t, func() {
		So(Glob("service:*").Pattern(), ShouldEqual, "*")
		So(Glob("???").Pattern(), ShouldEqual, "")
	})

	Convey("Match works", t, func() {
		trials := []struct {
			g      Glob
			id     Identity
			result bool
		}{
			{"user:abc@example.com", "user:abc@example.com", true},
			{"user:*@example.com", "user:abc@example.com", true},
			{"user:*", "user:abc@example.com", true},
			{"user:prefix-*", "user:prefix-zzz", true},
			{"user:prefix-*", "user:another-prefix-zzz", false},
			{"user:prefix-*-suffix", "user:prefix-zzz-suffix", true},
			{"user:prefix-*-suffix", "user:prefix-zzz-suffizzz", false},
			{"user:*", "user:\n", false},
			{"user:\n", "user:zzz", false},
			{"bad glob", "user:abc@example.com", false},
			{"user:*", "bad ident", false},
			{"user:*", "service:abc", false},
		}
		for _, entry := range trials {
			So(entry.g.Match(entry.id), ShouldEqual, entry.result)
		}
	})

	Convey("translate works", t, func() {
		trials := []struct {
			pat string
			reg string
		}{
			{"", `^$`},
			{"*", `^.*$`},
			{"abc", `^abc$`},
			{".?", `^\.\?$`},
			{"perfix-*@suffix", `^perfix-.*@suffix$`},
		}
		for _, entry := range trials {
			re, err := translate(entry.pat)
			So(err, ShouldBeNil)
			So(re, ShouldEqual, entry.reg)
		}
	})

	Convey("translate reject newline", t, func() {
		_, err := translate("blah\nblah")
		So(err, ShouldNotBeNil)
	})
}
