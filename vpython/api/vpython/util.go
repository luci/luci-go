// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package vpython

import (
	"strings"

	"github.com/golang/protobuf/proto"
)

// Version is a version string. It must be updated any time the text protobufs
// advance in a non-backwards-compatible way.
//
// This version string is used in the generation of filenames, and must be
// filesystem-compatible.
const Version = "v1"

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
func (t *Environment_Pep425Tag) IsZero() bool {
	return t == nil || (t.Version == "" && t.Abi == "" && t.Arch == "")
}

// TagString returns an underscore-separated string containing t's fields.
func (t *Environment_Pep425Tag) TagString() string {
	return strings.Join([]string{t.Version, t.Abi, t.Arch}, "-")
}

// HasABI returns true if t declares that it only works with a specific ABI.
func (t *Environment_Pep425Tag) HasABI() bool { return t.Abi != "none" }

// AnyArch returns true if t declares that it works on any architecture.
func (t *Environment_Pep425Tag) AnyArch() bool { return t.Arch == "any" }

// Count returns the number of populated fields in this tag.
func (t *Environment_Pep425Tag) Count() (v int) {
	if t.HasABI() {
		v++
	}
	if t.AnyArch() {
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
