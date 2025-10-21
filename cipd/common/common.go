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

package common

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"path"
	"regexp"
	"sort"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common/cipderr"
)

var (
	// packageNameRe is a regular expression for a superset of a set of allowed
	// package names.
	//
	// Package names must be lower case and have form "<word>/<word/<word>". See
	// ValidatePackageName for the full spec of how the package name can look.
	//
	// Note: do NOT ever add '+' as allowed character. It will break various URL
	// parsers that use '/+/' to separate parameters.
	packageNameRe = regexp.MustCompile(`^([a-z0-9_\-\.]+/)*[a-z0-9_\-\.]+$`)

	// A regular expression for a tag key.
	tagKeyReStr = `^[a-z0-9_\-]+$`
	tagKeyRe    = regexp.MustCompile(tagKeyReStr)

	// A regular expression for tag values.
	//
	// Basically printable ASCII (plus space), except symbols that have meaning in
	// command line or URL contexts (!"#$%&?'^|).
	//
	// Additionally, spaces are allowed only inside the value, not as a prefix or
	// a suffix.
	tagValReStr = `^[A-Za-z0-9$()*+,\-./:;<=>@\\_{}~ ]+$`
	tagValRe    = regexp.MustCompile(tagValReStr)

	// packageRefRe is a regular expression for a ref.
	packageRefReStr = `^[a-z0-9_./\-]{1,256}$`
	packageRefRe    = regexp.MustCompile(packageRefReStr)

	// Parameters for instance metadata key-value validation.
	metadataKeyMaxLen = 400
	metadataKeyReStr  = `^[a-z0-9_\-]+$`
	metadataKeyRe     = regexp.MustCompile(metadataKeyReStr)
)

// MetadataMaxLen is maximum allowed length of an instance metadata entry body.
const MetadataMaxLen = 512 * 1024

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
// at the end (such trailing '/' is stripped by this function), and it can be
// an empty string or "/" (to indicate the root of the repository).
func ValidatePackagePrefix(p string) (string, error) {
	p = strings.TrimSuffix(p, "/")
	if p != "" {
		if err := validatePathishString(p, "package prefix"); err != nil {
			return "", err
		}
	}
	return p, nil
}

// validatePathishString is common implementation of ValidatePackageName and
// ValidatePackagePrefix.
func validatePathishString(p, title string) error {
	if !packageNameRe.MatchString(p) {
		return validationErr("invalid %s %q: must be a slash-separated path where each component matches \"[a-z0-9_\\-\\.]+\"", title, p)
	}
	for _, chunk := range strings.Split(p, "/") {
		if strings.Count(chunk, ".") == len(chunk) {
			return validationErr("invalid %s %q: dots-only path components are forbidden", title, p)
		}
	}
	return nil
}

// ValidatePin returns error if package name or instance id are invalid.
func ValidatePin(pin Pin, v HashAlgoValidation) error {
	if err := ValidatePackageName(pin.PackageName); err != nil {
		return err
	}
	return ValidateInstanceID(pin.InstanceID, v)
}

// ValidatePackageRef returns error if a string doesn't look like a valid ref.
func ValidatePackageRef(r string) error {
	if ValidateInstanceID(r, AnyHash) == nil {
		return validationErr("invalid ref name %q: it looks like an instance ID causing ambiguities", r)
	}
	if !packageRefRe.MatchString(r) {
		return validationErr("invalid ref name %q: must match %q", r, packageRefReStr)
	}
	return nil
}

// ValidateInstanceTag returns error if a string doesn't look like a valid tag.
func ValidateInstanceTag(t string) error {
	_, err := ParseInstanceTag(t)
	return err
}

// ParseInstanceTag takes "k:v" string and returns its proto representation.
func ParseInstanceTag(t string) (*repopb.Tag, error) {
	switch chunks := strings.SplitN(t, ":", 2); {
	case len(chunks) != 2:
		return nil, validationErr("%q doesn't look like a tag (a key:value pair)", t)
	case len(t) > 400:
		return nil, validationErr("the tag is too long, should be <=400 chars: %q", t)
	case !tagKeyRe.MatchString(chunks[0]):
		return nil, validationErr("invalid tag key in %q: should match %q", t, tagKeyReStr)
	case strings.HasPrefix(chunks[1], " ") || strings.HasSuffix(chunks[1], " "):
		return nil, validationErr("invalid tag value in %q: should not start or end with ' '", t)
	case !tagValRe.MatchString(chunks[1]):
		return nil, validationErr("invalid tag value in %q: should match %q", t, tagValReStr)
	default:
		return &repopb.Tag{
			Key:   chunks[0],
			Value: chunks[1],
		}, nil
	}
}

// MustParseInstanceTag takes "k:v" string returns its proto representation or
// panics if the tag is invalid.
func MustParseInstanceTag(t string) *repopb.Tag {
	tag, err := ParseInstanceTag(t)
	if err != nil {
		panic(err)
	}
	return tag
}

// JoinInstanceTag returns "k:v" representation of the tag.
//
// Doesn't validate it.
func JoinInstanceTag(t *repopb.Tag) string {
	return t.Key + ":" + t.Value
}

// ValidateInstanceVersion return error if a string can't be used as version.
//
// A version can be specified as:
//  1. Instance ID (hash, e.g. "1234deadbeef2234...").
//  2. Package ref (e.g. "latest").
//  3. Instance tag (e.g. "git_revision:abcdef...").
func ValidateInstanceVersion(v string) error {
	if ValidateInstanceID(v, AnyHash) == nil ||
		ValidatePackageRef(v) == nil ||
		ValidateInstanceTag(v) == nil {
		return nil
	}
	return validationErr("bad version %q: not an instance ID, a ref or a tag", v)
}

// ValidateSubdir returns an error if the string can't be used as an ensure-file
// subdir.
func ValidateSubdir(subdir string) error {
	if subdir == "" { // empty is fine
		return nil
	}
	if strings.Contains(subdir, "\\") {
		return validationErr(`bad subdir %q: backslashes are not allowed (use "/")`, subdir)
	}
	if strings.Contains(subdir, ":") {
		return validationErr(`bad subdir %q: colons are not allowed`, subdir)
	}
	if cleaned := path.Clean(subdir); cleaned != subdir {
		return validationErr("bad subdir %q: should be simplified to %q", subdir, cleaned)
	}
	if strings.HasPrefix(subdir, "./") || strings.HasPrefix(subdir, "../") || subdir == "." {
		return validationErr(`bad subdir %q: contains disallowed dot-path prefix`, subdir)
	}
	if strings.HasPrefix(subdir, "/") {
		return validationErr("bad subdir %q: absolute paths are not allowed", subdir)
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
		return validationErr("%q doesn't look like a principal id (<type>:<id>)", p)
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
func NormalizePrefixMetadata(m *repopb.PrefixMetadata) error {
	var err error
	if m.Prefix, err = ValidatePackagePrefix(m.Prefix); err != nil {
		return err
	}

	// There should be only one ACL section per role.
	perRole := make(map[repopb.Role]*repopb.PrefixMetadata_ACL, len(m.Acls))
	keys := make([]int, 0, len(perRole))
	for i, acl := range m.Acls {
		switch {
		// Note: we allow roles not currently present in *.proto, maybe they came
		// from a newer server. 0 is never OK though.
		case acl.Role == 0:
			return validationErr("ACL entry #%d doesn't have a role specified", i)
		case perRole[acl.Role] != nil:
			return validationErr("role %s is specified twice", acl.Role)
		}

		perRole[acl.Role] = acl
		keys = append(keys, int(acl.Role))

		sort.Strings(acl.Principals)
		for _, p := range acl.Principals {
			if err := ValidatePrincipalName(p); err != nil {
				return validationErr("in ACL entry for role %s: %s", acl.Role, err)
			}
		}
	}

	// Sort ACLs by role.
	if len(keys) != len(m.Acls) {
		panic("must not happen")
	}
	sort.Ints(keys)
	for i, role := range keys {
		m.Acls[i] = perRole[repopb.Role(role)]
	}

	return nil
}

// PinSlice is a simple list of Pins
type PinSlice []Pin

// Validate ensures that this PinSlice contains no duplicate packages or invalid
// pins.
func (s PinSlice) Validate(v HashAlgoValidation) error {
	dedup := stringset.New(len(s))
	for _, p := range s {
		if err := ValidatePin(p, v); err != nil {
			return err
		}
		if !dedup.Add(p.PackageName) {
			return validationErr("duplicate package %q", p.PackageName)
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
func (p PinSliceBySubdir) Validate(v HashAlgoValidation) error {
	for subdir, pkgs := range p {
		if err := ValidateSubdir(subdir); err != nil {
			return err
		}
		if err := pkgs.Validate(v); err != nil {
			return validationErr("subdir %q: %s", subdir, err)
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

// ValidateInstanceMetadataKey returns an error if the given key can't be used
// as an instance metadata key.
func ValidateInstanceMetadataKey(key string) error {
	if len(key) > metadataKeyMaxLen {
		return validationErr("invalid metadata key %q: too long, should be <=%d chars", key, metadataKeyMaxLen)
	}
	if !metadataKeyRe.MatchString(key) {
		return validationErr("invalid metadata key %q: should match %q", key, metadataKeyReStr)
	}
	return nil
}

// ValidateInstanceMetadataLen returns an error if the given length of the
// metadata payload is too large.
func ValidateInstanceMetadataLen(l int) error {
	if l > MetadataMaxLen {
		return validationErr("the metadata value is too long: should be <=%d bytes, got %d", MetadataMaxLen, l)
	}
	return nil
}

// ValidateContentType returns an error if the given string can't be used as
// an instance metadata content type.
func ValidateContentType(ct string) error {
	if ct == "" {
		return nil
	}
	if len(ct) > 400 {
		return validationErr("the content type is too long: should be <=400 bytes, got %d", len(ct))
	}
	_, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return validationErr("bad content type %q: %s", ct, err)
	}
	return nil
}

// ValidateInstanceMetadataFingerprint returns an error if the given string
// doesn't look like an output of InstanceMetadataFingerprint.
func ValidateInstanceMetadataFingerprint(fp string) error {
	if len(fp) != 32 {
		return validationErr("bad metadata fingerprint %q: expecting 32 hex chars", fp)
	}
	if err := checkIsHex(fp); err != nil {
		return validationErr("bad metadata fingerprint %q: %s", fp, err)
	}
	return nil
}

// InstanceMetadataFingerprint calculates a fingerprint of an instance metadata
// entry.
//
// Doesn't do any validation.
func InstanceMetadataFingerprint(key string, value []byte) string {
	h := sha256.New()
	h.Write([]byte(key))
	h.Write([]byte{':'})
	h.Write(value)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

// validationErr returns a tagged validation error.
func validationErr(format string, args ...any) error {
	return grpcutil.InvalidArgumentTag.Apply(
		cipderr.BadArgument.Apply(
			errors.Fmt(format, args...)))
}
