// Copyright 2017 The LUCI Authors.
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

package vpython

import (
	"strings"

	"google.golang.org/protobuf/proto"
)

// Version is a version string. It must be updated any time the text protobufs
// advance in a non-backwards-compatible way.
//
// This version string is used in the generation of filenames, and must be
// filesystem-compatible.
const Version = "v2"

// Clone returns a deep clone of the supplied Environment.
//
// If e is nil, a non-nil empty Environment will be returned.
func (e *Environment) Clone() *Environment {
	if e == nil {
		return &Environment{}
	}
	return proto.Clone(e).(*Environment)
}

// IsZero returns true if this tag is a zero value.
func (t *PEP425Tag) IsZero() bool {
	return t == nil || (t.Python == "" && t.Abi == "" && t.Platform == "")
}

// TagString returns an underscore-separated string containing t's fields.
func (t *PEP425Tag) TagString() string {
	return strings.Join([]string{t.Python, t.Abi, t.Platform}, "-")
}

// HasABI returns true if t declares that it only works with a specific ABI.
func (t *PEP425Tag) HasABI() bool { return t.Abi != "none" }

// AnyPlatform returns true if t declares that it works on any platform.
func (t *PEP425Tag) AnyPlatform() bool { return t.Platform == "any" }

// Count returns the number of populated fields in this tag.
func (t *PEP425Tag) Count() (v int) {
	if t.HasABI() {
		v++
	}
	if t.AnyPlatform() {
		v++
	}
	return
}

// Clone returns a deep clone of the supplied Spec.
//
// If e is nil, a non-nil empty Spec will be returned.
func (s *Spec) Clone() *Spec {
	if s == nil {
		return &Spec{}
	}
	return proto.Clone(s).(*Spec)
}
