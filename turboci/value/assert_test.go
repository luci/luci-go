// Copyright 2026 The LUCI Authors.
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

package value

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

// assertNoErr fails the test fatally if err is not nil.
func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// assertErrLike fails the test fatally if err is nil or does not contain the
// expected substring.
func assertErrLike(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("Expected error containing %q, got: %v", substr, err)
	}
}

// assertEqual fails the test if got != want (using simple comparison).
func assertEqual[T comparable](t *testing.T, want, got T) {
	t.Helper()
	if got != want {
		t.Errorf("Mismatch:\nwant: %v\ngot:  %v", want, got)
	}
}

// assertMatch fails the test if got and want do not match.
// It uses go-cmp and automatically handles proto messages correctly.
// Additional cmp.Options can be passed (e.g., protocmp.IgnoreUnknown()).
func assertMatch(t *testing.T, want, got any, opts ...cmp.Option) {
	t.Helper()
	allOpts := append([]cmp.Option{protocmp.Transform()}, opts...)
	if diff := cmp.Diff(want, got, allOpts...); diff != "" {
		t.Errorf("Mismatch (-want +got):\n%s", diff)
	}
}

// assertTrue fails the test if val is false.
func assertTrue(t *testing.T, val bool) {
	t.Helper()
	if !val {
		t.Error("Expected true, got false")
	}
}

// assertFalse fails the test if val is true.
func assertFalse(t *testing.T, val bool) {
	t.Helper()
	if val {
		t.Error("Expected false, got true")
	}
}

// assertNil fails the test if val is not nil.
func assertNil(t *testing.T, val any) {
	t.Helper()
	if !isNil(val) {
		t.Errorf("Expected nil, got: %v", val)
	}
}

// assertLen fails the test if the length of got is not want.
func assertLen[T any](t *testing.T, got []T, want int) {
	t.Helper()
	if len(got) != want {
		t.Errorf("Expected length %d, got %d: %v", want, len(got), got)
	}
}

// assertEmpty fails the test if got is not empty.
func assertEmpty[T any](t *testing.T, got []T) {
	t.Helper()
	if len(got) != 0 {
		t.Errorf("Expected empty, got length %d: %v", len(got), got)
	}
}

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer,
		reflect.UnsafePointer, reflect.Interface, reflect.Slice:

		return v.IsNil()
	}
	return false
}
