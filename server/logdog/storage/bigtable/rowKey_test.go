// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRowKey(t *testing.T) {
	t.Parallel()

	Convey(`A row key, constructed from "a/b/+/c/d"`, t, func() {
		path := "a/b/+/c/d"
		rk := newRowKey(path, 1337, 42)

		Convey(`Shares a path with a row key from the same Path.`, func() {
			So(rk.sharesPathWith(newRowKey(path, 2468, 0)), ShouldBeTrue)
		})

		for _, v := range []string{
			"a/b/+/c",
			"asdf",
			"",
		} {
			Convey(fmt.Sprintf(`Does not share a path with: %q`, v), func() {
				So(rk.sharesPathWith(newRowKey(v, 0, 0)), ShouldBeFalse)
			})
		}

		Convey(`Can be encoded, then decoded into its fields.`, func() {
			enc := rk.encode()
			So(len(enc), ShouldBeLessThanOrEqualTo, maxEncodedKeySize)

			drk, err := decodeRowKey(enc)
			So(err, ShouldBeNil)

			So(drk.pathHash, ShouldResemble, rk.pathHash)
			So(drk.index, ShouldEqual, rk.index)
			So(drk.count, ShouldEqual, rk.count)
		})
	})

	Convey(`A series of ordered row keys`, t, func() {
		prev := ""
		for _, i := range []int64{
			-1, /* Why not? */
			0,
			7,
			8,
			257,
			1029,
			1337,
		} {
			Convey(fmt.Sprintf(`Row key %d should be ascendingly sorted and parsable.`, i), func() {
				rk := newRowKey("test", i, i)

				// Test that it encodes/decodes back to identity.
				enc := rk.encode()
				drk, err := decodeRowKey(enc)
				So(err, ShouldBeNil)
				So(drk.index, ShouldEqual, i)

				// Assert that it is ordered.
				if prev != "" {
					So(prev, ShouldBeLessThan, enc)

					prevp, err := decodeRowKey(prev)
					So(err, ShouldBeNil)
					So(prevp.sharesPathWith(rk), ShouldBeTrue)
					So(prevp.index, ShouldBeLessThan, drk.index)
					So(prevp.count, ShouldBeLessThan, drk.count)
				}

				Convey(`Legacy row key value will parse with count 0.`, func() {
					if rk.count > 0 {
						enc = enc[:strings.LastIndex(enc, "~")]
					}

					drk, err = decodeRowKey(enc)
					So(err, ShouldBeNil)
					So(drk.index, ShouldEqual, i)
					So(drk.count, ShouldEqual, 0)

					// Assert that it is ordered.
					if prev != "" {
						So(prev, ShouldBeLessThan, enc)

						prevp, err := decodeRowKey(prev)
						So(err, ShouldBeNil)
						So(prevp.sharesPathWith(rk), ShouldBeTrue)
						So(prevp.index, ShouldBeLessThan, drk.index)
					}
				})
			})
		}
	})

	Convey(`Invalid row keys will fail to decode with "errMalformedRowKey".`, t, func() {
		for _, t := range []struct {
			name string
			v    string
		}{
			{"No tilde", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd38080"},
			{"No path hash", "~8080"},
			{"Bad hex path hash", "badhex~8080"},
			{"Path has too short", "4a54700540127~8080"},
			{"Bad hex index", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3~badhex"},
			{"Missing index.", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3~"},
			{"Varint overflow", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3~ffffffffffff"},
			{"Trailing data", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3~8080badd06"},
		} {
			Convey(fmt.Sprintf(`Row key fails to decode [%s]: %q`, t.name, t.v), func() {
				_, err := decodeRowKey(t.v)
				So(err, ShouldEqual, errMalformedRowKey)
			})
		}
	})
}
