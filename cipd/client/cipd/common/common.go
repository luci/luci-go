// Copyright 2014 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package common defines structures and functions used by cipd/* packages.
package common

import (
	"fmt"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/luci/luci-go/common/data/stringset"
)

// packageNameRe is a regular expression for a package name: <word>/<word/<word>
// Package names must be lower case.
var packageNameRe = regexp.MustCompile(`^([a-z0-9_\-]+/)*[a-z0-9_\-]+$`)

// instanceTagKeyRe is a regular expression for a tag key.
var instanceTagKeyRe = regexp.MustCompile(`^[a-z0-9_\-]+$`)

// packageRefRe is a regular expression for a ref.
var packageRefRe = regexp.MustCompile(`^[a-z0-9_\-]{1,100}$`)

// Pin uniquely identifies an instance of some package.
type Pin struct {
	PackageName string `json:"package"`
	InstanceID  string `json:"instance_id"`
}

// String converts pin to a human readable string.
func (pin Pin) String() string {
	return fmt.Sprintf("%s:%s", pin.PackageName, pin.InstanceID)
}

// ValidatePackageName returns error if a string isn't a valid package name.
func ValidatePackageName(name string) error {
	if !packageNameRe.MatchString(name) {
		return fmt.Errorf("invalid package name: %s", name)
	}
	return nil
}

// ValidateInstanceID returns error if a string isn't a valid instance id.
func ValidateInstanceID(s string) error {
	// Instance id is SHA1 hex digest currently.
	if len(s) != 40 {
		return fmt.Errorf("not a valid package instance ID %q: not 40 bytes", s)
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("not a valid package instance ID %q: wrong char %c", s, c)
		}
	}
	return nil
}

// ValidateFileHash returns error if a string isn't a valid exe hash.
func ValidateFileHash(s string) error {
	// file hashes are SHA1 hex digests currently.
	if len(s) != 40 {
		return fmt.Errorf("not a valid exe hash %q: not 40 bytes", s)
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("not a valid exe hash %q: wrong char %c", s, c)
		}
	}
	return nil
}

// ValidatePin returns error if package name or instance id are invalid.
func ValidatePin(pin Pin) error {
	if err := ValidatePackageName(pin.PackageName); err != nil {
		return err
	}
	if err := ValidateInstanceID(pin.InstanceID); err != nil {
		return err
	}
	return nil
}

// ValidatePackageRef returns error if a string doesn't look like a valid ref.
func ValidatePackageRef(r string) error {
	if ValidateInstanceID(r) == nil {
		return fmt.Errorf("invalid ref name (looks like an instance ID): %q", r)
	}
	if !packageRefRe.MatchString(r) {
		return fmt.Errorf("invalid ref name: %q", r)
	}
	return nil
}

// ValidateInstanceTag returns error if a string doesn't look like a valid tag.
func ValidateInstanceTag(t string) error {
	chunks := strings.SplitN(t, ":", 2)
	if len(chunks) != 2 {
		return fmt.Errorf("%q doesn't look like a tag (a key:value pair)", t)
	}
	if len(t) > 400 {
		return fmt.Errorf("the tag is too long: %q", t)
	}
	if !instanceTagKeyRe.MatchString(chunks[0]) {
		return fmt.Errorf("invalid tag key in %q (should be a lowercase word)", t)
	}
	return nil
}

// ValidateInstanceVersion return error if a string can't be used as version.
//
// A version can be specified as:
//  1) Instance ID (hash, e.g "1234deadbeef2234...").
//  2) Package ref (e.g. "latest").
//  3) Instance tag (e.g. "git_revision:abcdef...").
func ValidateInstanceVersion(v string) error {
	if ValidateInstanceID(v) == nil || ValidatePackageRef(v) == nil || ValidateInstanceTag(v) == nil {
		return nil
	}
	return fmt.Errorf("bad version (not an instance ID, a ref or a tag): %q", v)
}

// ValidateRoot returns an error if the string can't be used as an ensure-file
// root.
func ValidateRoot(root string) error {
	if root == "" { // empty is fine
		return nil
	}
	if strings.Contains(root, "\\") {
		return fmt.Errorf(`bad root path: backslashes not allowed (use "/"): %q`, root)
	}
	if strings.Contains(root, ":") {
		return fmt.Errorf(`bad root path: colons are not allowed: %q`, root)
	}
	if cleaned := path.Clean(root); cleaned != root {
		return fmt.Errorf("bad root path: %q (should be %q)", root, cleaned)
	}
	if strings.HasPrefix(root, "./") || strings.HasPrefix(root, "../") || root == "." {
		return fmt.Errorf(`bad root path: invalid ".": %q`, root)
	}
	if strings.HasPrefix(root, "/") {
		return fmt.Errorf("bad root path: absolute paths not allowed: %q", root)
	}
	return nil
}

// GetInstanceTagKey returns key portion of the instance tag or empty string.
func GetInstanceTagKey(t string) string {
	chunks := strings.SplitN(t, ":", 2)
	if len(chunks) != 2 {
		return ""
	}
	return chunks[0]
}

var currentArchitecture = ""
var currentPlatform = ""

func init() {
	// TODO(iannucci): rationalize these to just be exactly GOOS and GOARCH.
	currentArchitecture = runtime.GOOS
	if currentArchitecture == "darwin" {
		currentArchitecture = "mac"
	}

	currentPlatform = runtime.GOARCH
	if currentPlatform == "arm" {
		currentPlatform = "armv6l"
	}
}

// CurrentArchitecture returns the current cipd-style architecture that the
// current go binary conforms to. Possible values:
//   - "armv6l" (if GOARCH=arm)
//   - other GOARCH values
func CurrentArchitecture() string {
	return currentArchitecture
}

// CurrentPlatform returns the current cipd-style platform that the
// current go binary conforms to. Possible values:
//   - "mac" (if GOOS=darwin)
//   - other GOOS values
func CurrentPlatform() string {
	return currentPlatform
}

// PinSlice is a simple list of Pins
type PinSlice []Pin

// Validate ensures that this PinSlice contains no duplicate packages or invalid
// pins.
func (s PinSlice) Validate() error {
	dedup := stringset.New(len(s))
	for _, p := range s {
		if err := ValidatePin(p); err != nil {
			return err
		}
		if !dedup.Add(p.PackageName) {
			return fmt.Errorf("duplicate package %q", p.PackageName)
		}
	}
	return nil
}

// ToMap converts the PinSlice to a PinMap.
func (s PinSlice) ToMap() PinMap {
	ret := make(PinMap, len(s))
	for _, p := range s {
		ret[p.PackageName] = p.InstanceID
	}
	return ret
}

// PinMap is a map of package_name to instanceID.
type PinMap map[string]string

// ToSlice converts the PinMap to a PinSlice.
func (m PinMap) ToSlice() PinSlice {
	s := make(PinSlice, 0, len(m))
	pkgs := make(sort.StringSlice, 0, len(m))
	for k := range m {
		pkgs = append(pkgs, k)
	}
	pkgs.Sort()
	for _, pkg := range pkgs {
		s = append(s, Pin{pkg, m[pkg]})
	}
	return s
}

// PinSliceByRoot is a simple mapping of root path to pin.
type PinSliceByRoot map[string]PinSlice

// Validate ensures that this doesn't contain any invalid
// root paths, duplicate packages within the same root, or invalid pins.
func (p PinSliceByRoot) Validate() error {
	for root, pkgs := range p {
		if err := ValidateRoot(root); err != nil {
			return err
		}
		if err := pkgs.Validate(); err != nil {
			return fmt.Errorf("root %q: %s", root, err)
		}
	}
	return nil
}

// ToMap converts this to a PinMapByRoot
func (p PinSliceByRoot) ToMap() PinMapByRoot {
	ret := make(PinMapByRoot, len(p))
	for root, pkgs := range p {
		ret[root] = pkgs.ToMap()
	}
	return ret
}

// PinMapByRoot is a simple mapping of root -> package_name -> instanceID
type PinMapByRoot map[string]PinMap

// ToSlice converts this to a PinSliceByRoot
func (p PinMapByRoot) ToSlice() PinSliceByRoot {
	ret := make(PinSliceByRoot, len(p))
	for root, pkgs := range p {
		ret[root] = pkgs.ToSlice()
	}
	return ret
}
