// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/luci/luci-go/vpython/api/vpython"

	"github.com/luci/luci-go/common/data/sortby"
	"github.com/luci/luci-go/common/errors"

	"github.com/golang/protobuf/proto"
)

// Clone returns a deep copy clone of the provided spec.
//
// If spec is nil, an empty vpython.Spec will be returned.
func Clone(spec *vpython.Spec) *vpython.Spec {
	if spec == nil {
		return &vpython.Spec{}
	}
	return proto.Clone(spec).(*vpython.Spec)
}

// Normalize normalizes the specification Message such that two messages
// with identical meaning will have identical representation.
func Normalize(spec *vpython.Spec) error {
	sort.Sort(specPackageSlice(spec.Wheel))

	// No duplicate packages. Since we're sorted, we can just check for no
	// immediate repetitions.
	for i, pkg := range spec.Wheel {
		if i > 0 && pkg.Name == spec.Wheel[i-1].Name {
			return errors.Reason("duplicate spec entries for package %(path)q").
				D("name", pkg.Name).
				Err()
		}
	}

	return nil
}

// Hash hashes the contents of the supplied "spec" and returns the result as
// a hex-encoded string.
func Hash(spec *vpython.Spec) string {
	data, err := proto.Marshal(spec)
	if err != nil {
		panic(fmt.Errorf("failed to marshal proto: %v", err))
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

type specPackageSlice []*vpython.Spec_Package

func (s specPackageSlice) Len() int      { return len(s) }
func (s specPackageSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s specPackageSlice) Less(i, j int) bool {
	return sortby.Chain{
		func(i, j int) bool { return s[i].Name < s[j].Name },
		func(i, j int) bool { return s[i].Version < s[j].Version },
	}.Use(i, j)
}
