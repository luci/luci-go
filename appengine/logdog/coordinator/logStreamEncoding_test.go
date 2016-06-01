// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package coordinator

import (
	"fmt"
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLogStreamEncoding(t *testing.T) {
	t.Parallel()

	for _, s := range []string{
		"",
		"hello",
		"from the outside",
		"+-#$!? \t\n",
	} {
		Convey(fmt.Sprintf(`Can encode: %q`, s), t, func() {
			enc := encodeKey(s)
			So(enc, ShouldStartWith, encodedKeyPrefix)

			Convey(`And then decode back into the original string.`, func() {
				dec, err := decodeKey(enc)
				So(err, ShouldBeNil)
				So(dec, ShouldEqual, s)
			})
		})
	}

	Convey(`Will fail to decode a string that doesn't begin with the encoded prefix.`, t, func() {
		_, err := decodeKey("foo")
		So(err, ShouldErrLike, "encoded key missing prefix")
	})

	Convey(`Will fail to decode a string that's not properly encoded.`, t, func() {
		_, err := decodeKey(encodedKeyPrefix + "!!!")
		So(err, ShouldErrLike, "failed to decode key")
	})
}
