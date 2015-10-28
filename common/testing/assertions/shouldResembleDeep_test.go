// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assertions

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShouldResembleV(t *testing.T) {
	Convey(`A struct with pointers`, t, func() {
		type Y struct {
			V string
		}

		type X struct {
			Y *Y
		}

		a := X{Y: &Y{V: "foo"}}

		Convey(`Should have a nice ShouldResembleV string.`, func() {
			b := X{Y: &Y{V: "bar"}}

			So(ShouldResembleV(a, b), ShouldEqual, ""+
				"Expected: 'assertions.X{Y:(*assertions.Y){V:\"bar\"}}'\n"+
				"Actual:   'assertions.X{Y:(*assertions.Y){V:\"foo\"}}'\n"+
				"(Should resemble)!")
		})

		Convey(`Should have a nice ShouldNotResembleV string.`, func() {
			So(ShouldNotResembleV(a, a), ShouldEqual, ""+
				"Expected        'assertions.X{Y:(*assertions.Y){V:\"foo\"}}'\n"+
				"to NOT resemble 'assertions.X{Y:(*assertions.Y){V:\"foo\"}}'\n"+
				"(but it did)!")
		})
	})

	Convey(`A protobuf-like struct`, t, func() {
		type FakeProtoMember struct {
			Key   string
			Value string
		}

		type FakeProto struct {
			Name    string
			Member  *FakeProtoMember
			Members []*FakeProtoMember
			Numbers []int
		}

		Convey(`Can be successfully deep-compared.`, func() {
			f := FakeProto{
				Name: "test",
				Member: &FakeProtoMember{
					Key:   "foo",
					Value: "bar",
				},
				Members: []*FakeProtoMember{
					{
						Key:   "baz",
						Value: "qux",
					},
					{
						Key:   "luci",
						Value: "rocks",
					},
				},
			}
			fc := f
			So(f, ShouldResembleV, f)

			Convey(`Will not resemble a clone with a modified Name.`, func() {
				fc.Name = "other"
				So(f, ShouldNotResembleV, fc)
			})

			Convey(`Will resemble a clone with a different inner pointer as long as its value matches.`, func() {
				fc.Members = []*FakeProtoMember{
					f.Members[0],
					{
						Key:   "luci",
						Value: "rocks",
					},
				}
				So(f.Members, ShouldNotEqual, fc.Members)
				So(f, ShouldResembleV, fc)
			})

			Convey(`Will not resemble a clone with a different inner value.`, func() {
				fc.Members = []*FakeProtoMember{
					f.Members[0],
					{
						Key:   "luci",
						Value: "rules",
					},
				}
				So(f.Members, ShouldNotEqual, fc.Members)
				So(f.Members, ShouldNotResembleV, fc.Members)
				So(f, ShouldNotResembleV, fc)
			})
		})
	})
}
