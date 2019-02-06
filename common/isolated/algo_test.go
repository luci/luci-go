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

package isolated

import (
	"bytes"
	"io/ioutil"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCompressor_flate(t *testing.T) {
	for _, namespace := range []string{"default-gzip", "sha1-flate", "sha256-gzip"} {
		t.Run(namespace, func(t *testing.T) {
			b := bytes.Buffer{}
			in, err := GetCompressor(namespace, &b)
			if err != nil {
				t.Fatal(err)
			}
			// Send highly repeated content.
			content := strings.Repeat("foobar", 100)
			if _, err := in.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
			if err := in.Close(); err != nil {
				t.Fatal(err)
			}
			if b.Len() >= len(content) {
				t.Fatalf("expected compression to happen: %d < %d", b.Len(), len(content))
			}
			out, err := GetDecompressor(namespace, &b)
			if err != nil {
				t.Fatal(err)
			}
			res, err := ioutil.ReadAll(out)
			if err != nil {
				t.Fatal(err)
			}
			if string(res) != content {
				t.Fatalf("%s != %s", content, res)
			}
		})
	}
}

func TestCompressor_flat(t *testing.T) {
	for _, namespace := range []string{"default-foo", "sha1-bar", "sha256-GCP"} {
		t.Run(namespace, func(t *testing.T) {
			b := bytes.Buffer{}
			in, err := GetCompressor(namespace, &b)
			if err != nil {
				t.Fatal(err)
			}
			// Send highly repeated content.
			content := strings.Repeat("foobar", 100)
			if _, err := in.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
			if err := in.Close(); err != nil {
				t.Fatal(err)
			}
			if b.Len() != len(content) {
				t.Fatalf("expected compression to not happen: %d != %d", b.Len(), len(content))
			}
			out, err := GetDecompressor(namespace, &b)
			if err != nil {
				t.Fatal(err)
			}
			res, err := ioutil.ReadAll(out)
			if err != nil {
				t.Fatal(err)
			}
			if string(res) != content {
				t.Fatalf("%s != %s", content, res)
			}
		})
	}
}

func TestHexDigestValid(t *testing.T) {
	data := []struct {
		v         HexDigest
		namespace string
	}{
		{"0123456789012345678901234567890123456789", "sha1-yeah"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "default-gzip"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "sha256-gzip"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "sha512-gzip"},
	}
	for i, line := range data {
		if !line.v.Validate(GetHash(line.namespace)) {
			t.Fatalf("#%d: expected success: %v", i, line)
		}
	}
}

func TestHexDigestInvalid(t *testing.T) {
	data := []struct {
		v         HexDigest
		namespace string
	}{
		{"012345678901234567890123456789012345678X", "sha1-yeah"},
		{"012345678901234567890123456789012345678", "sha1-yeah"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "default-gzip"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "sha256-gzip"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "sha512-gzip"},
	}
	for i, line := range data {
		if line.v.Validate(GetHash(line.namespace)) {
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
