// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package errors

import (
	stdErr "errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type customInt int
type customIntTag struct{ Key TagKey }

func (t customIntTag) With(i customInt) TagValue { return TagValue{t.Key, i} }
func (t customIntTag) In(err error) (v customInt, ok bool) {
	d, ok := TagValueIn(t.Key, err)
	if ok {
		v = d.(customInt)
	}
	return
}

var aCustomIntTag = customIntTag{NewTagKey("errors.testing custom int tag")}

type stringTag struct{ Key TagKey }

func (t stringTag) With(s string) TagValue { return TagValue{t.Key, s} }
func (t stringTag) In(err error) (v string, ok bool) {
	d, ok := TagValueIn(t.Key, err)
	if ok {
		v = d.(string)
	}
	return
}

var aStringTag = stringTag{NewTagKey("errors.testing string tag")}

func TestTags(t *testing.T) {
	t.Parallel()

	aBoolTag := BoolTag{NewTagKey("errors.testing tag")}

	Convey("Tags", t, func() {
		Convey(`have unique tagKey values`, func() {
			tagSet := map[TagKey]struct{}{}
			tagSet[aBoolTag.Key] = struct{}{}
			tagSet[aStringTag.Key] = struct{}{}
			tagSet[aCustomIntTag.Key] = struct{}{}
			So(tagSet, ShouldHaveLength, 3)
		})

		Convey(`can be applied to errors`, func() {
			Convey(`at creation time`, func() {
				err := New("I am an error", aBoolTag, aStringTag.With("hi"))
				So(aBoolTag.In(err), ShouldBeTrue)
				d, ok := aStringTag.In(err)
				So(ok, ShouldBeTrue)
				So(d, ShouldEqual, "hi")

				_, ok = aCustomIntTag.In(err)
				So(ok, ShouldBeFalse)
			})

			Convey(`added to existing errors`, func() {
				err := New("I am an error")
				err2 := aCustomIntTag.With(236).Apply(err)
				err2 = aBoolTag.Apply(err2)

				d, ok := aCustomIntTag.In(err2)
				So(ok, ShouldBeTrue)
				So(d, ShouldEqual, customInt(236))
				So(aBoolTag.In(err2), ShouldBeTrue)

				_, ok = aCustomIntTag.In(err)
				So(ok, ShouldBeFalse)
				So(aBoolTag.In(err), ShouldBeFalse)
			})

			Convey(`added to stdlib errors`, func() {
				err := stdErr.New("I am an error")
				err2 := aStringTag.With("hi").Apply(err)

				d, ok := aStringTag.In(err2)
				So(ok, ShouldBeTrue)
				So(d, ShouldEqual, "hi")

				_, ok = aStringTag.In(err)
				So(ok, ShouldBeFalse)
			})

			Convey(`multiple applications has the last one win`, func() {
				err := New("I am an error")
				err = aStringTag.With("hi").Apply(err)
				err = aStringTag.With("there").Apply(err)
				err = aStringTag.With("winner").Apply(err)

				d, ok := aStringTag.In(err)
				So(ok, ShouldBeTrue)
				So(d, ShouldEqual, "winner")

				Convey(`muliterrors are first to last`, func() {
					err = NewMultiError(
						New("a", aStringTag.With("hi"), aBoolTag),
						New("b", aCustomIntTag.With(10), aStringTag.With("no")),
						New("c", aCustomIntTag.With(20), aStringTag.With("nopers")),
					)

					So(aBoolTag.In(err), ShouldBeTrue)

					d, ok := aStringTag.In(err)
					So(ok, ShouldBeTrue)
					So(d, ShouldEqual, "hi")

					ci, ok := aCustomIntTag.In(err)
					So(ok, ShouldBeTrue)
					So(ci, ShouldEqual, customInt(10))

					Convey(`and all the correct values show up with GetTags`, func() {
						tags := GetTags(err)
						So(tags, ShouldContainKey, aStringTag.Key)
						So(tags, ShouldContainKey, aBoolTag.Key)

						So(tags[aCustomIntTag.Key], ShouldEqual, 10)
						So(tags[aStringTag.Key], ShouldEqual, "hi")
						So(tags[aBoolTag.Key], ShouldEqual, true)
					})
				})
			})
		})
	})
}
