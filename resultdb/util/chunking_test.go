// Copyright 2024 The LUCI Authors.
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

package util

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSplitToChunks(t *testing.T) {
	Convey("chunk content", t, func() {
		Convey("empty string, 0 chunk", func() {
			content := []byte("")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(len(chunks), ShouldEqual, 0)
		})
		Convey("1 character, 1 chunk", func() {
			content := []byte("a")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"a"})
		})
		Convey("maxSize character, 1 chunk", func() {
			content := []byte("0123456789")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"0123456789"})
		})
		Convey("No delimiter", func() {
			content := []byte("0123456789abcdefghij")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"0123456789", "abcdefghij"})
		})
		Convey("No delimiter 1", func() {
			content := []byte("0123456789abcdefghij0")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"0123456789", "abcdefghij", "0"})
		})
		Convey("No delimiter 2", func() {
			content := []byte("0123456789abcdefghij0123456789a")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"0123456789", "abcdefghij", "0123456789", "a"})
		})
		Convey("Delimiter \r\n", func() {
			content := []byte("012\n34\r\n789a bcd\r\n")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"012\n34\r\n", "789a bcd\r\n"})
		})
		Convey("Delimiter \r\n lookback window size 1", func() {
			content := []byte("012\n34\r\n789a bcd\r\n")
			chunks, err := SplitToChunks(content, 10, 1)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"012\n34\r\n78", "9a bcd\r\n"})
		})
		Convey("Delimiter \n or \r", func() {
			content := []byte("012\r 345\n6 789 ab\n\ncdef\ng\rhij012")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"012\r 345\n", "6 789 ab\n\n", "cdef\ng\r", "hij012"})
		})
		Convey("Delimiter \n or \r lookback window size 1", func() {
			content := []byte("012\r 345\n6 789 ab\n\ncdef\ng\rhij012")
			chunks, err := SplitToChunks(content, 10, 1)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"012\r 345\n6", " 789 ab\n\nc", "def\ng\rhij0", "12"})
		})
		Convey("Whitespace", func() {
			content := []byte("012 345\t6789a\tbc dfe gh ij 01234\t56 789acbdefghij")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"012 345\t", "6789a\tbc ", "dfe gh ij ", "01234\t56 ", "789acbdefg", "hij"})
		})
		Convey("Whitespace lookback window size 1", func() {
			content := []byte("012 345\t6789a\tbc dfe gh ij 01234\t56 789abcdefghij")
			chunks, err := SplitToChunks(content, 10, 1)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"012 345\t67", "89a\tbc dfe", " gh ij 012", "34\t56 789a", "bcdefghij"})
		})
		Convey("Invalid unicode character", func() {
			content := []byte{0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88}
			_, err := SplitToChunks(content, 10, 0)
			So(err, ShouldNotBeNil)
		})
		Convey("Should not break unicode character", func() {
			// € sign is 3 bytes 0xE2, 0x82, 0xAC in UTF-8.
			content := []byte{0xE2, 0x82, 0xAC, 0xE2, 0x82, 0xAC, 0xE2, 0x82, 0xAC, 0xE2, 0x82, 0xAC}
			chunks, err := SplitToChunks(content, 10, 0)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"€€€", "€"})
		})
		Convey("Should not break unicode character 1", func() {
			// €  is 3 bytes 0xE2, 0x82, 0xAC in UTF-8.
			// 𐍈 is 4 bytes: 0xF0, 0x90, 0x8D, 0x88.
			// Ç is 2 bytes: 0xC3 0x87
			content := []byte{0xF0, 0x90, 0x8D, 0x88, 0xF0, 0x90, 0x8D, 0x88, 0xE2, 0x82, 0xAC, 0xF0, 0x90, 0x8D, 0x88, 0xC3, 0x87, 0xC3, 0x87}
			chunks, err := SplitToChunks(content, 10, 0)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"𐍈𐍈", "€𐍈Ç", "Ç"})
		})
		Convey("Unicode and whitespace and linebreak", func() {
			content := []byte("𐍈 𐍈Ç ÇÇ Ç Ç ÇÇa\r\nbcde")
			chunks, err := SplitToChunks(content, 10, 10)
			So(err, ShouldBeNil)
			So(chunks, ShouldResemble, []string{"𐍈 ", "𐍈Ç ", "ÇÇ Ç ", "Ç ÇÇa\r\n", "bcde"})
		})
	})
}
