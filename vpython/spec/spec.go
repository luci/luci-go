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

// Render creates a human-readable string from spec.
func Render(spec *vpython.Spec) string { return proto.MarshalTextString(spec) }

// Normalize normalizes the specification Message such that two messages
// with identical meaning will have identical representation.
func Normalize(spec *vpython.Spec, defaultVENVPackage *vpython.Spec_Package) error {
	if spec.Virtualenv == nil {
		spec.Virtualenv = defaultVENVPackage
	}

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
//
// If not empty, the contents of extra are prefixed to hash string. This can
// be used to factor additional influences into the spec hash.
func Hash(spec *vpython.Spec, extra string) string {
	data, err := proto.Marshal(spec)
	if err != nil {
		panic(fmt.Errorf("failed to marshal proto: %v", err))
	}

	mustWrite := func(v int, err error) {
		if err != nil {
			panic(fmt.Errorf("impossible: %s", err))
		}
	}

	hash := sha256.New()
	if extra != "" {
		mustWrite(fmt.Fprintf(hash, "%s:", extra))
	}
	mustWrite(fmt.Fprintf(hash, "%s:", vpython.Version))
	mustWrite(hash.Write(data))
	return hex.EncodeToString(hash.Sum(nil))
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
