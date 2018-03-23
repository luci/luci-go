// Copyright 2014 The LUCI Authors.
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

// Package common defines structures and functions used by cipd/* packages.
package common

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// packageNameRe is a regular expression for a superset of a set of allowed
// package names.
//
// Package names must be lower case and have form "<word>/<word/<word>". See
// ValidatePackageName for the full spec of how the package name can look.
var packageNameRe = regexp.MustCompile(`^([a-z0-9_\-\.]+/)*[a-z0-9_\-\.]+$`)

// instanceTagKeyRe is a regular expression for a tag key.
var instanceTagKeyRe = regexp.MustCompile(`^[a-z0-9_\-]+$`)

// packageRefRe is a regular expression for a ref.
var packageRefRe = regexp.MustCompile(`^[a-z0-9_./\-]{1,256}$`)

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
	return validatePathishString(name, "package name")
}

// ValidatePackagePrefix normalizes and validates a package prefix.
//
// A prefix is basically like a package name, except it is allowed to have '/'
// at the end. Such trailing '/' is stripped by this function.
func ValidatePackagePrefix(p string) (string, error) {
	p = strings.TrimSuffix(p, "/")
	if err := validatePathishString(p, "package prefix"); err != nil {
		return "", err
	}
	return p, nil
}

// validatePathishString is common implementation of ValidatePackageName and
// ValidatePackagePrefix.
func validatePathishString(p, title string) error {
	if !packageNameRe.MatchString(p) {
		return fmt.Errorf("invalid %s: %q", title, p)
	}
	for _, chunk := range strings.Split(p, "/") {
		if strings.Count(chunk, ".") == len(chunk) {
			return fmt.Errorf("invalid %s (dots-only names are forbidden): %q", title, p)
		}
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
	return ValidateInstanceID(pin.InstanceID)
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

// ValidateSubdir returns an error if the string can't be used as an ensure-file
// subdir.
func ValidateSubdir(subdir string) error {
	if subdir == "" { // empty is fine
		return nil
	}
	if strings.Contains(subdir, "\\") {
		return fmt.Errorf(`bad subdir: backslashes not allowed (use "/"): %q`, subdir)
	}
	if strings.Contains(subdir, ":") {
		return fmt.Errorf(`bad subdir: colons are not allowed: %q`, subdir)
	}
	if cleaned := path.Clean(subdir); cleaned != subdir {
		return fmt.Errorf("bad subdir: %q (should be %q)", subdir, cleaned)
	}
	if strings.HasPrefix(subdir, "./") || strings.HasPrefix(subdir, "../") || subdir == "." {
		return fmt.Errorf(`bad subdir: invalid ".": %q`, subdir)
	}
	if strings.HasPrefix(subdir, "/") {
		return fmt.Errorf("bad subdir: absolute paths not allowed: %q", subdir)
	}
	return nil
}

// ValidatePrincipalName validates strings used to identify principals in ACLs.
//
// The expected format is "<key>:<value>" pair, where <key> is one of "group",
// "user", "anonymous", "service". See also go.chromium.org/luci/auth/identity.
func ValidatePrincipalName(p string) error {
	chunks := strings.Split(p, ":")
	if len(chunks) != 2 || chunks[0] == "" || chunks[1] == "" {
		return fmt.Errorf("%q doesn't look like principal id (<type>:<id>)", p)
	}
	if chunks[0] == "group" {
		return nil // any non-empty group name is OK
	}
	// Should be valid identity otherwise.
	_, err := identity.MakeIdentity(p)
	return err
}

// NormalizePrefixMetadata validates and normalizes the prefix metadata proto.
//
// Updates r.Prefix in-place by stripping trailing '/', sorts r.Acls and
// principals lists inside them. Skips r.Fingerprint, r.UpdateTime and
// r.UpdateUser, since they are always overridden on the server side.
func NormalizePrefixMetadata(m *api.PrefixMetadata) error {
	var err error
	if m.Prefix, err = ValidatePackagePrefix(m.Prefix); err != nil {
		return err
	}

	// There should be only one ACL section per role.
	perRole := make(map[api.Role]*api.PrefixMetadata_ACL, len(m.Acls))
	keys := make([]int, 0, len(perRole))
	for i, acl := range m.Acls {
		switch {
		// Note: we allow roles not currently present in *.proto, maybe they came
		// from a newer server. 0 is never OK though.
		case acl.Role == 0:
			return fmt.Errorf("ACL entry #%d doesn't have a role specified", i)
		case perRole[acl.Role] != nil:
			return fmt.Errorf("role %s is specified twice", acl.Role)
		}

		perRole[acl.Role] = acl
		keys = append(keys, int(acl.Role))

		sort.Strings(acl.Principals)
		for _, p := range acl.Principals {
			if err := ValidatePrincipalName(p); err != nil {
				return fmt.Errorf("in ACL entry for role %s - %s", acl.Role, err)
			}
		}
	}

	// Sort ACLs by role.
	if len(keys) != len(m.Acls) {
		panic("must not happen")
	}
	sort.Ints(keys)
	for i, role := range keys {
		m.Acls[i] = perRole[api.Role(role)]
	}

	return nil
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

// PinSliceBySubdir is a simple mapping of subdir to pin slice.
type PinSliceBySubdir map[string]PinSlice

// Validate ensures that this doesn't contain any invalid
// subdirs, duplicate packages within the same subdir, or invalid pins.
func (p PinSliceBySubdir) Validate() error {
	for subdir, pkgs := range p {
		if err := ValidateSubdir(subdir); err != nil {
			return err
		}
		if err := pkgs.Validate(); err != nil {
			return fmt.Errorf("subdir %q: %s", subdir, err)
		}
	}
	return nil
}

// ToMap converts this to a PinMapBySubdir
func (p PinSliceBySubdir) ToMap() PinMapBySubdir {
	ret := make(PinMapBySubdir, len(p))
	for subdir, pkgs := range p {
		ret[subdir] = pkgs.ToMap()
	}
	return ret
}

// PinMapBySubdir is a simple mapping of subdir -> package_name -> instanceID
type PinMapBySubdir map[string]PinMap

// ToSlice converts this to a PinSliceBySubdir
func (p PinMapBySubdir) ToSlice() PinSliceBySubdir {
	ret := make(PinSliceBySubdir, len(p))
	for subdir, pkgs := range p {
		ret[subdir] = pkgs.ToSlice()
	}
	return ret
}
