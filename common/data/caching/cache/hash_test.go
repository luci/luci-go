// Copyright 2021 The LUCI Authors.
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

package cache

import (
	"bytes"
	"crypto"
	"reflect"
	"sort"
	"testing"
)

func TestHexDigestValid(t *testing.T) {
	data := []struct {
		v HexDigest
		h crypto.Hash
	}{
		{"0123456789012345678901234567890123456789", crypto.SHA1},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", crypto.SHA1},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", crypto.SHA256},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", crypto.SHA512},
	}
	for i, line := range data {
		if !line.v.Validate(line.h) {
			t.Fatalf("#%d: expected success: %v", i, line)
		}
	}
}

func TestHexDigestInvalid(t *testing.T) {
	data := []struct {
		v HexDigest
		h crypto.Hash
	}{
		{"012345678901234567890123456789012345678X", crypto.SHA1},
		{"012345678901234567890123456789012345678", crypto.SHA1},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", crypto.SHA1},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", crypto.SHA256},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", crypto.SHA512},
	}
	for i, line := range data {
		if line.v.Validate(line.h) {
			t.Fatalf("#%d: expected failure: %v", i, line)
		}
	}
}

func TestHexDigests(t *testing.T) {
	v := HexDigests{"a", "c", "b", "d"}
	sort.Sort(v)
	if !reflect.DeepEqual(v, HexDigests{"a", "b", "c", "d"}) {
		t.Fatal("sort failed")
	}
}

func TestSum(t *testing.T) {
	h := crypto.SHA1.New()
	if _, err := h.Write([]byte("foo")); err != nil {
		t.Fatal(err)
	}
	expected := HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33")
	if s := Sum(h); s != expected {
		t.Fatalf("%s != %s", s, expected)
	}
}

func TestHash(t *testing.T) {
	b := bytes.NewBufferString("foo")
	s, err := Hash(crypto.SHA1, b)
	if err != nil {
		t.Fatal(err)
	}
	expected := HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33")
	if s != expected {
		t.Fatalf("%s != %s", s, expected)
	}
}

func TestHashBytes(t *testing.T) {
	s := HashBytes(crypto.SHA1, []byte("foo"))
	expected := HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33")
	if s != expected {
		t.Fatalf("%s != %s", s, expected)
	}
}
