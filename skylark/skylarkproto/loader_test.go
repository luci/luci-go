// Copyright 2018 The LUCI Authors.
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

package skylarkproto

import (
	"testing"

	"github.com/google/skylark"
	"github.com/google/skylark/skylarkstruct"
)

func TestLoaderWorks(t *testing.T) {
	t.Parallel()

	dict, err := LoadProtoModule("go.chromium.org/luci/skylark/skylarkproto/testprotos/test.proto")
	if err != nil {
		t.Fatalf("failed to load test.proto - %s", err)
	}
	if len(dict) != 1 {
		t.Fatalf("expecting 1 item in the dict, got 2: %v", dict)
	}
	symbols, ok := dict["testprotos"].(*skylarkstruct.Struct)
	if !ok {
		t.Fatalf("%v is not *skylarkstruct.Struct", dict["testprotos"])
	}

	// This dict contains all top-level symbols discovered in the proto file.
	// The test will break and will need to be updated whenever something new is
	// added to test.proto. Note that symbols from another.proto do not appear
	// here (by design).
	dict = skylark.StringDict{}
	symbols.ToStringDict(dict)
	expected := map[string]struct{}{
		"Complex":       {},
		"IntFields":     {},
		"MessageFields": {},
		"Simple":        {},
	}
	for k := range dict {
		if _, ok := expected[k]; !ok {
			t.Errorf("unexpected exported symbol %q", k)
		}
	}
	for k := range expected {
		if _, ok := dict[k]; !ok {
			t.Errorf("expected symbol %q is not exported", k)
		}
	}
}

func TestLoaderUnknownProto(t *testing.T) {
	t.Parallel()

	_, err := LoadProtoModule("unknown.proto")
	if err == nil {
		t.Fatalf("LoadProtoModule unexpectedly succeeded")
	}
	if err.Error() != "no such proto file registered" {
		t.Fatalf("bad error messages: %s", err)
	}
}
