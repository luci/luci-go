// Copyright 2015 The LUCI Authors.
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

package cmpbin

import (
	"bytes"
	"io"
	"math/rand"
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBytes(t *testing.T) {
	t.Parallel()

	ftt.Run("bytes", t, func(t *ftt.Test) {
		b := &bytes.Buffer{}
		tc := []byte("this is a test")

		t.Run("good", func(t *ftt.Test) {
			_, err := WriteBytes(b, tc)
			assert.Loosely(t, err, should.BeNil)
			exn := b.Len()
			r, n, err := ReadBytes(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(exn))
			assert.Loosely(t, r, should.Resemble(tc))
		})

		t.Run("bad (truncated buffer)", func(t *ftt.Test) {
			_, err := WriteBytes(b, tc)
			assert.Loosely(t, err, should.BeNil)
			bs := b.Bytes()[:b.Len()-4]
			_, n, err := ReadBytes(bytes.NewBuffer(bs))
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, n, should.Equal(len(bs)))
		})

		t.Run("bad (bad varint)", func(t *ftt.Test) {
			_, n, err := ReadBytes(bytes.NewBuffer([]byte{0b10001111}))
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, n, should.Equal(1))
		})

		t.Run("bad (huge data)", func(t *ftt.Test) {
			n, err := WriteBytes(b, make([]byte, 2*1024*1024+1))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(ReadByteLimit+2))
			_, n, err = ReadBytes(b)
			assert.Loosely(t, err.Error(), should.ContainSubstring("too big!"))
			assert.Loosely(t, n, should.Equal(ReadByteLimit))
		})

		t.Run("bad (write errors)", func(t *ftt.Test) {
			_, err := WriteBytes(&fakeWriter{1}, tc)
			assert.Loosely(t, err.Error(), should.ContainSubstring("nope"))

			// transition boundary
			_, err = WriteBytes(&fakeWriter{7}, tc)
			assert.Loosely(t, err.Error(), should.ContainSubstring("nope"))
		})
	})
}

func TestStrings(t *testing.T) {
	t.Parallel()

	ftt.Run("strings", t, func(t *ftt.Test) {
		b := &bytes.Buffer{}

		t.Run("empty", func(t *ftt.Test) {
			n, err := WriteString(b, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(1))
			assert.Loosely(t, b.Bytes(), should.Resemble([]byte{0}))
			r, n, err := ReadString(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(1))
			assert.Loosely(t, r, should.BeEmpty)
		})

		t.Run("nulls", func(t *ftt.Test) {
			s := "\x00\x00\x00\x00\x00"
			n, err := WriteString(b, s)
			assert.Loosely(t, n, should.Equal(6))
			assert.Loosely(t, err, should.BeNil)
			exn := b.Len()
			r, n, err := ReadString(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(exn))
			assert.Loosely(t, r, should.Equal(s))
		})

		t.Run("bad (truncated buffer)", func(t *ftt.Test) {
			s := "this is a test"
			n, err := WriteString(b, s)
			assert.Loosely(t, n, should.Equal((len(s)*8)/7+1))
			assert.Loosely(t, err, should.BeNil)
			bs := b.Bytes()[:b.Len()-1]
			_, n, err = ReadString(bytes.NewBuffer(bs))
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, n, should.Equal(len(bs)))
		})

		t.Run("single", func(t *ftt.Test) {
			n, err := WriteString(b, "1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))
			assert.Loosely(t, b.Bytes(), should.Resemble([]byte{0b00110001, 0b10000000}))
			r, n, err := ReadString(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))
			assert.Loosely(t, r, should.Equal("1"))
		})

		t.Run("good", func(t *ftt.Test) {
			s := "this is a test"
			n, err := WriteString(b, s)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal((len(s)*8)/7+1))
			assert.Loosely(t, b.Bytes()[:8], should.Resemble([]byte{
				0b01110101, 0b00110101, 0b00011011, 0b00101111, 0b00110011, 0b00000011,
				0b10100101, 0b11100111,
			}))
			exn := b.Len()
			r, n, err := ReadString(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(r), should.Equal(len(s)))
			assert.Loosely(t, n, should.Equal(exn))
			assert.Loosely(t, r, should.Equal(s))
		})

	})
}

// TODO(riannucci): make it [][]string instead

func TestStringSortability(t *testing.T) {
	t.Parallel()

	ftt.Run("strings maintain sort order", t, func(t *ftt.Test) {
		special := []string{
			"",
			"\x00",
			"\x00\x00",
			"\x00\x00\x00",
			"\x00\x00\x00\x00",
			"\x00\x00\x00\x00\x00",
			"\x00\x00\x00\x00\x01",
			"\x00\x00\x00\x00\xFF",
			"1234567",
			"12345678",
			"123456789",
			string(make([]byte, 7*8)),
		}
		orig := make(sort.StringSlice, randomTestSize, randomTestSize+len(special))

		r := rand.New(rand.NewSource(*seed))
		for i := range orig {
			buf := make([]byte, r.Intn(100))
			for j := range buf {
				buf[j] = byte(r.Uint32()) // watch me not care!
			}
			orig[i] = string(buf)
		}
		orig = append(orig, special...)

		enc := make(sort.StringSlice, len(orig))
		b := &bytes.Buffer{}
		for i := range enc {
			b.Reset()
			_, err := WriteString(b, orig[i])
			assert.Loosely(t, err, should.BeNil)
			enc[i] = b.String()
		}

		orig.Sort()
		enc.Sort()

		for i := range orig {
			decoded, _, err := ReadString(bytes.NewBufferString(enc[i]))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, decoded, should.Resemble(orig[i]))
		}
	})
}

type StringSliceSlice [][]string

func (s StringSliceSlice) Len() int      { return len(s) }
func (s StringSliceSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s StringSliceSlice) Less(i, j int) bool {
	a, b := s[i], s[j]
	lim := len(a)
	if len(b) < lim {
		lim = len(b)
	}
	for k := 0; k < lim; k++ {
		if a[k] > b[k] {
			return false
		} else if a[k] < b[k] {
			return true
		}
	}
	return len(a) < len(b)
}

func TestConcatenatedStringSortability(t *testing.T) {
	t.Parallel()

	ftt.Run("concatenated strings maintain sort order", t, func(t *ftt.Test) {
		orig := make(StringSliceSlice, randomTestSize)

		r := rand.New(rand.NewSource(*seed))
		for i := range orig {
			count := r.Intn(10)
			for j := 0; j < count; j++ {
				buf := make([]byte, r.Intn(100))
				for j := range buf {
					buf[j] = byte(r.Uint32()) // watch me not care!
				}
				orig[i] = append(orig[i], string(buf))
			}
		}
		orig = append(orig, [][]string{
			nil,
			{"", "aaa"},
			{"a", "aa"},
			{"aa", "a"},
		}...)

		enc := make(sort.StringSlice, len(orig))
		b := &bytes.Buffer{}
		for i, slice := range orig {
			b.Reset()
			for _, s := range slice {
				_, err := WriteString(b, s)
				assert.Loosely(t, err, should.BeNil)
			}
			enc[i] = b.String()
		}

		sort.Sort(orig)
		enc.Sort()

		for i := range orig {
			decoded := []string(nil)
			buf := bytes.NewBufferString(enc[i])
			for {
				dec, _, err := ReadString(buf)
				if err != nil {
					break
				}
				decoded = append(decoded, dec)
			}
			assert.Loosely(t, decoded, should.Resemble(orig[i]))
		}
	})
}
