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
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
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

// GetInstanceTagKey returns key portion of the instance tag or empty string.
func GetInstanceTagKey(t string) string {
	chunks := strings.SplitN(t, ":", 2)
	if len(chunks) != 2 {
		return ""
	}
	return chunks[0]
}

// HashForInstanceID constructs correct zero hash.Hash instance that can be
// used to verify given instance ID.
//
// Currently it is always SHA1.
//
// TODO(vadimsh): Use this function where sha1.New() is used now.
func HashForInstanceID(instanceID string) (hash.Hash, error) {
	if err := ValidateInstanceID(instanceID); err != nil {
		return nil, err
	}
	return sha1.New(), nil
}

// InstanceIDFromHash returns an instance ID string, given a hash state.
func InstanceIDFromHash(h hash.Hash) string {
	return hex.EncodeToString(h.Sum(nil))
}

// DefaultHash returns a zero hash.Hash instance to use for package verification
// by default.
//
// Currently it is always SHA1.
//
// TODO(vadimsh): Use this function where sha1.New() is used now.
func DefaultHash() hash.Hash {
	return sha1.New()
}

var currentArchitecture = ""
var currentOS = ""

func init() {
	// TODO(iannucci): rationalize these to just be exactly GOOS and GOARCH.
	currentArchitecture = runtime.GOARCH
	if currentArchitecture == "arm" {
		currentArchitecture = "armv6l"
	}

	currentOS = runtime.GOOS
	if currentOS == "darwin" {
		currentOS = "mac"
	}
}

// CurrentArchitecture returns the current cipd-style architecture that the
// current go binary conforms to. Possible values:
//   - "armv6l" (if GOARCH=arm)
//   - other GOARCH values
func CurrentArchitecture() string {
	return currentArchitecture
}

// CurrentOS returns the current cipd-style os that the
// current go binary conforms to. Possible values:
//   - "mac" (if GOOS=darwin)
//   - other GOOS values
func CurrentOS() string {
	return currentOS
}

// CurrentPlatform returns the current platform.
func CurrentPlatform() TemplatePlatform {
	return TemplatePlatform{currentOS, currentArchitecture}
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

// TemplateExpander is a mapping of simple string substitutions which is used to
// expand cipd package name templates. For example:
//
//   ex, err := TemplateExpander{
//     "platform": "mac-amd64"
//   }.Expand("foo/${platform}")
//
// `ex` would be "foo/mac-amd64".
//
// Use DefaultPackageNameExpander() to obtain the default mapping for CIPD
// applications.
type TemplateExpander map[string]string

// ErrSkipTemplate may be returned from TemplateExpander.Expand to indicate that
// a given expansion doesn't apply to the current template parameters. For
// example, expanding `"foo/${os=linux,mac}"` with a template parameter of "os"
// == "win", would return ErrSkipTemplate.
var ErrSkipTemplate = errors.New("package template does not apply to the current system")

var templateParm = regexp.MustCompile(`\${[^}]*}`)

// Expand applies package template expansion rules to the package template,
//
// If err == ErrSkipTemplate, that means that this template does not apply to
// this os/arch combination and should be skipped.
//
// The expansion rules are as follows:
//   - "some text" will pass through unchanged
//   - "${variable}" will directly substitute the given variable
//   - "${variable=val1,val2}" will substitute the given variable, if its value
//     matches one of the values in the list of values. If the current value
//     does not match, this returns ErrSkipTemplate.
//
// Attempting to expand an unknown variable is an error.
// After expansion, any lingering '$' in the template is an error.
func (t TemplateExpander) Expand(template string) (pkg string, err error) {
	return t.expandImpl(template, false)
}

// Validate returns an error if this template doesn't appear to be valid given
// the current TemplateExpander parameters.
//
// This will catch issues like malformed template parameters and unknown
// variables, and will replace all ${param=value} items with the first item in
// the value list, even if the current TemplateExpander value doesn't match.
//
// This is mostly used for validating user input when the correct values of
// TemplateExpander aren't known yet.
func (t TemplateExpander) Validate(template string) (pkg string, err error) {
	return t.expandImpl(template, true)
}

func (t TemplateExpander) expandImpl(template string, alwaysFill bool) (pkg string, err error) {
	skip := false

	pkg = templateParm.ReplaceAllStringFunc(template, func(parm string) string {
		// ${...}
		contents := parm[2 : len(parm)-1]

		varNameValues := strings.SplitN(contents, "=", 2)
		if len(varNameValues) == 1 {
			// ${varName}
			if value, ok := t[varNameValues[0]]; ok {
				return value
			}

			err = errors.Reason("unknown variable in ${%s}", contents).Err()
		}

		// ${varName=value,value}
		ourValue, ok := t[varNameValues[0]]
		if !ok {
			err = errors.Reason("unknown variable %q", parm).Err()
			return parm
		}

		for _, val := range strings.Split(varNameValues[1], ",") {
			if val == ourValue || alwaysFill {
				return ourValue
			}
		}
		skip = true
		return parm
	})
	if skip {
		err = ErrSkipTemplate
	}
	if err == nil && strings.ContainsRune(pkg, '$') {
		err = errors.Reason("unable to process some variables in %q", template).Err()
	}
	return
}

// TemplatePlatform contains the parameters for a "${platform}" template.
//
// The string value can be obtained by calling String().
// be parsed using ParseTemplatePlatform.
type TemplatePlatform struct {
	OS   string
	Arch string
}

// ParseTemplatePlatform parses a TemplatePlatform from its string
// representation.
func ParseTemplatePlatform(v string) (TemplatePlatform, error) {
	parts := strings.Split(v, "-")
	if len(parts) != 2 {
		return TemplatePlatform{}, errors.Reason("platform must be <os>-<arch>: %q", v).Err()
	}
	return TemplatePlatform{parts[0], parts[1]}, nil
}

func (tp TemplatePlatform) String() string {
	return fmt.Sprintf("%s-%s", tp.OS, tp.Arch)
}

// Expander returns a TemplateExpander populated with tp's fields.
func (tp TemplatePlatform) Expander() TemplateExpander {
	return TemplateExpander{
		"os":       tp.OS,
		"arch":     tp.Arch,
		"platform": tp.String(),
	}
}

// DefaultTemplateExpander returns the default template expander.
//
// This has values populated for ${os}, ${arch} and ${platform}.
func DefaultTemplateExpander() TemplateExpander {
	return CurrentPlatform().Expander()
}
