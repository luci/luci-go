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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSplitToChunks(t *testing.T) {
	ftt.Run("chunk content", t, func(t *ftt.Test) {
		t.Run("empty string, 0 chunk", func(t *ftt.Test) {
			content := []byte("")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(chunks), should.BeZero)
		})
		t.Run("1 character, 1 chunk", func(t *ftt.Test) {
			content := []byte("a")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"a"}))
		})
		t.Run("maxSize character, 1 chunk", func(t *ftt.Test) {
			content := []byte("0123456789")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"0123456789"}))
		})
		t.Run("No delimiter", func(t *ftt.Test) {
			content := []byte("0123456789abcdefghij")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"0123456789", "abcdefghij"}))
		})
		t.Run("No delimiter 1", func(t *ftt.Test) {
			content := []byte("0123456789abcdefghij0")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"0123456789", "abcdefghij", "0"}))
		})
		t.Run("No delimiter 2", func(t *ftt.Test) {
			content := []byte("0123456789abcdefghij0123456789a")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"0123456789", "abcdefghij", "0123456789", "a"}))
		})
		t.Run("Delimiter \r\n", func(t *ftt.Test) {
			content := []byte("012\n34\r\n789a bcd\r\n")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"012\n34\r\n", "789a bcd\r\n"}))
		})
		t.Run("Delimiter \r\n lookback window size 1", func(t *ftt.Test) {
			content := []byte("012\n34\r\n789a bcd\r\n")
			chunks, err := SplitToChunks(content, 10, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"012\n34\r\n78", "9a bcd\r\n"}))
		})
		t.Run("Delimiter \n or \r", func(t *ftt.Test) {
			content := []byte("012\r 345\n6 789 ab\n\ncdef\ng\rhij012")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"012\r 345\n", "6 789 ab\n\n", "cdef\ng\r", "hij012"}))
		})
		t.Run("Delimiter \n or \r lookback window size 1", func(t *ftt.Test) {
			content := []byte("012\r 345\n6 789 ab\n\ncdef\ng\rhij012")
			chunks, err := SplitToChunks(content, 10, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"012\r 345\n6", " 789 ab\n\nc", "def\ng\rhij0", "12"}))
		})
		t.Run("Whitespace", func(t *ftt.Test) {
			content := []byte("012 345\t6789a\tbc dfe gh ij 01234\t56 789acbdefghij")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"012 345\t", "6789a\tbc ", "dfe gh ij ", "01234\t56 ", "789acbdefg", "hij"}))
		})
		t.Run("Whitespace lookback window size 1", func(t *ftt.Test) {
			content := []byte("012 345\t6789a\tbc dfe gh ij 01234\t56 789abcdefghij")
			chunks, err := SplitToChunks(content, 10, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"012 345\t67", "89a\tbc dfe", " gh ij 012", "34\t56 789a", "bcdefghij"}))
		})
		t.Run("Invalid unicode character", func(t *ftt.Test) {
			content := []byte{0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88}
			_, err := SplitToChunks(content, 10, 0)
			assert.Loosely(t, err, should.NotBeNil)
		})
		t.Run("Should not break unicode character", func(t *ftt.Test) {
			// € sign is 3 bytes 0xE2, 0x82, 0xAC in UTF-8.
			content := []byte{0xE2, 0x82, 0xAC, 0xE2, 0x82, 0xAC, 0xE2, 0x82, 0xAC, 0xE2, 0x82, 0xAC}
			chunks, err := SplitToChunks(content, 10, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"€€€", "€"}))
		})
		t.Run("Should not break unicode character 1", func(t *ftt.Test) {
			// €  is 3 bytes 0xE2, 0x82, 0xAC in UTF-8.
			// 𐍈 is 4 bytes: 0xF0, 0x90, 0x8D, 0x88.
			// Ç is 2 bytes: 0xC3 0x87
			content := []byte{0xF0, 0x90, 0x8D, 0x88, 0xF0, 0x90, 0x8D, 0x88, 0xE2, 0x82, 0xAC, 0xF0, 0x90, 0x8D, 0x88, 0xC3, 0x87, 0xC3, 0x87}
			chunks, err := SplitToChunks(content, 10, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"𐍈𐍈", "€𐍈Ç", "Ç"}))
		})
		t.Run("Unicode and whitespace and linebreak", func(t *ftt.Test) {
			content := []byte("𐍈 𐍈Ç ÇÇ Ç Ç ÇÇa\r\nbcde")
			chunks, err := SplitToChunks(content, 10, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, chunks, should.Match([]string{"𐍈 ", "𐍈Ç ", "ÇÇ Ç ", "Ç ÇÇa\r\n", "bcde"}))
		})
	})
}
