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

// NormalizeEnvironment normalizes the supplied Environment such that two
// messages with identical meaning will have identical representation.
func NormalizeEnvironment(env *vpython.Environment) error {
	if env.Spec == nil {
		env.Spec = &vpython.Spec{}
	}
	if err := NormalizeSpec(env.Spec, env.Pep425Tag); err != nil {
		return err
	}

	if env.Runtime == nil {
		env.Runtime = &vpython.Runtime{}
	}

	sort.Sort(pep425TagSlice(env.Pep425Tag))
	return nil
}

// NormalizeSpec normalizes the specification Message such that two messages
// with identical meaning will have identical representation.
//
// NormalizeSpec will prune any Wheel entries that don't match the specified
// tags, and will remove the match entries from any remaining Wheel entries.
func NormalizeSpec(spec *vpython.Spec, tags []*vpython.PEP425Tag) error {
	if spec.Virtualenv != nil && len(spec.Virtualenv.MatchTag) > 0 {
		// The VirtualEnv package may not specify a match tag.
		spec.Virtualenv.MatchTag = nil
	}

	// Apply match filters, prune any entries that don't match, and clear the
	// MatchTag entries for those that do.
	pos := 0
	for _, wheel := range spec.Wheel {
		if !PackageMatches(wheel, tags) {
			continue
		}

		wheel.MatchTag = nil
		spec.Wheel[pos] = wheel
		pos++
	}
	spec.Wheel = spec.Wheel[:pos]

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

// Hash hashes the contents of the supplied "spec" and "rt" and returns the
// result as a hex-encoded string.
//
// If not empty, the contents of extra are prefixed to hash string. This can
// be used to factor additional influences into the spec hash.
func Hash(spec *vpython.Spec, rt *vpython.Runtime, extra string) string {
	mustMarshal := func(msg proto.Message) []byte {
		data, err := proto.Marshal(msg)
		if err != nil {
			panic(fmt.Errorf("failed to marshal proto: %v", err))
		}
		return data
	}
	specData := mustMarshal(spec)
	rtData := mustMarshal(rt)

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
	mustWrite(hash.Write(specData))
	mustWrite(hash.Write([]byte(":")))
	mustWrite(hash.Write(rtData))
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

type pep425TagSlice []*vpython.PEP425Tag

func (s pep425TagSlice) Len() int      { return len(s) }
func (s pep425TagSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s pep425TagSlice) Less(i, j int) bool {
	return sortby.Chain{
		func(i, j int) bool { return s[i].Python < s[j].Python },
		func(i, j int) bool { return s[i].Abi < s[j].Abi },
		func(i, j int) bool { return s[i].Platform < s[j].Platform },
	}.Use(i, j)
}
