// Copyright 2019 The LUCI Authors.
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
	"crypto"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"go.chromium.org/luci/common/api/isolate/isolateservice/v1"
)

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

func TestHashFile(t *testing.T) {
	f, err := ioutil.TempFile("", "isolated")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(f.Name()); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := f.WriteString("foo"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := HashFile(crypto.SHA1, f.Name())
	if err != nil {
		t.Fatal(err)
	}
	expected := isolateservice.HandlersEndpointsV1Digest{
		Digest: "0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33",
		Size:   3,
	}
	if !reflect.DeepEqual(s, expected) {
		t.Fatalf("%v != %v", s, expected)
	}
}
