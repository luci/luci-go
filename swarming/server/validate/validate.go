// Copyright 2023 The LUCI Authors.
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

// Package validate contains validation for RPC requests and Config entries.
package validate

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
)

var cipdExpander = template.DefaultExpander()

const (
	maxDimensionKeyLen = 64
	maxDimensionValLen = 256
	// Max length of a valid service account.
	maxServiceAccountLength = 128
	// Maximum acceptable priority value, which is effectively the lowest priority.
	maxPriority = 255
	// If no update is received from bot after max seconds lapsed from its last
	// ping to the server, it will be considered dead.
	maxBotPingTolanceSecs = 1200
	// Min time to keep the bot alive before it is declared dead.
	minBotPingTolanceSecs = 60
	maxPubsubTopicLength  = 1024

	maxCacheCount      = 32
	maxCacheNameLength = 128
	maxCachePathLength = 256

	maxCIPDPackageCount  = 64
	MaxPackagePathLength = 256

	maxEnvVarLength   = 64
	MaxEnvValueLength = 1024
	MaxEnvVarCount    = 64
)

var (
	dimensionKeyRe = regexp.MustCompile(`^[a-zA-Z\-\_\.][0-9a-zA-Z\-\_\.]*$`)
	reservedTags   = stringset.NewFromSlice([]string{"swarming.terminate"}...)

	// cloudProjectIDRE is the cloud project identifier regex derived from
	// https://cloud.google.com/resource-manager/docs/creating-managing-projects#before_you_begin
	cloudProjectIDRE = regexp.MustCompile(`^[a-z]([a-z0-9-]){4,28}[a-z0-9]$`)
	// topicNameRE is the full topic name regex derived from https://cloud.google.com/pubsub/docs/admin#resource_names
	topicNameRE = regexp.MustCompile(`^projects/(.*)/topics/(.*)$`)
	// topicIDRE is the topic id regex derived from https://cloud.google.com/pubsub/docs/admin#resource_names
	topicIDRE   = regexp.MustCompile(`^[A-Za-z]([0-9A-Za-z\._\-~+%]){3,255}$`)
	cacheNameRe = regexp.MustCompile(`^[a-z0-9_]+$`)
	envVarRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// DimensionKey checks if `key` can be a dimension key.
func DimensionKey(key string) error {
	if err := keyLength(key); err != nil {
		return err
	}
	if !dimensionKeyRe.MatchString(key) {
		return errors.Reason("the key should match %s", dimensionKeyRe).Err()
	}
	return nil
}

func keyLength(key string) error {
	if key == "" {
		return errors.Reason("the key cannot be empty").Err()
	}
	if len(key) > maxDimensionKeyLen {
		return errors.Reason("the key should be no longer than %d (got %d)", maxDimensionKeyLen, len(key)).Err()
	}
	return nil
}

// DimensionValue checks if `val` can be a dimension value.
func DimensionValue(val string) error {
	if val == "" {
		return errors.Reason("the value cannot be empty").Err()
	}
	if len(val) > maxDimensionValLen {
		return errors.Reason("the value should be no longer than %d (got %d)", maxDimensionValLen, len(val)).Err()
	}
	if strings.TrimSpace(val) != val {
		return errors.Reason("the value should have no leading or trailing spaces").Err()
	}
	return nil
}

// CipdPackageName checks CIPD package name is correct.
//
// The package name is allowed to be a template e.g. have "${platform}" and
// other substitutions inside.
func CipdPackageName(pkg string) error {
	if pkg == "" {
		return errors.New("required")
	}
	expanded, err := cipdExpander.Expand(pkg)
	if err != nil {
		return errors.Annotate(err, "bad package name template %q", pkg).Err()
	}
	if err := common.ValidatePackageName(expanded); err != nil {
		return err // the error has all details already
	}
	return nil
}

// CipdPackageVersion checks CIPD package version is correct.
func CipdPackageVersion(ver string) error {
	if ver == "" {
		return errors.New("required")
	}
	return common.ValidateInstanceVersion(ver)
}

type cipdPkg interface {
	GetPath() string
	GetVersion() string
}

// CIPDPackages checks a slice of CIPD packages are correct.
func CIPDPackages[P cipdPkg](packages []P, requirePinnedVer bool, cachePaths stringset.Set) errors.MultiError {
	var merr errors.MultiError
	if len(packages) > maxCIPDPackageCount {
		merr.MaybeAdd(errors.Reason("can have up to %d packages", maxCIPDPackageCount).Err())
	}

	type pathName struct {
		p    string
		name string
	}
	pkgPathNames := make(map[pathName]struct{}, len(packages))
	for i, pkg := range packages {
		pkgName, err := packageName(pkg)
		if err != nil {
			merr.MaybeAdd(errors.Annotate(err, "name").Err())
		}

		subMerr := validateCIPDPackage(pkgName, pkg, requirePinnedVer, cachePaths)
		if subMerr.AsError() != nil {
			for _, err := range subMerr.Unwrap() {
				merr.MaybeAdd(errors.Annotate(err, "package %d (%s)", i, pkgName).Err())
			}
		}

		pn := pathName{pkg.GetPath(), pkgName}
		if _, ok := pkgPathNames[pn]; ok {
			merr.MaybeAdd(errors.Reason("package %q is specified more than once in path %s", pkgName, pkg.GetPath()).Err())
		} else {
			pkgPathNames[pn] = struct{}{}
		}
	}
	return merr
}

func validateCIPDPackage(pkgName string, pkg cipdPkg, requirePinnedVer bool, cachePaths stringset.Set) errors.MultiError {
	var merr errors.MultiError

	if err := CipdPackageName(pkgName); err != nil {
		merr.MaybeAdd(errors.Annotate(err, "name").Err())
	}

	if err := CipdPackageVersion(pkg.GetVersion()); err != nil {
		merr.MaybeAdd(errors.Annotate(err, "version").Err())
	}

	if err := Path(pkg.GetPath(), MaxPackagePathLength); err != nil {
		merr.MaybeAdd(errors.Annotate(err, "path").Err())
	}

	if cachePaths.Has(pkg.GetPath()) {
		merr.MaybeAdd(errors.Reason(
			"path %q is mapped to a named cache and cannot be a target of CIPD installation",
			pkg.GetPath()).Err())
	}
	if requirePinnedVer {
		if err := validatePinnedInstanceVersion(pkg.GetVersion()); err != nil {
			merr.MaybeAdd(errors.New(
				"an idempotent task cannot have unpinned packages; use tags or instance IDs as package versions"))
		}
	}
	return merr
}

func packageName(pkg cipdPkg) (string, error) {
	switch pkg := pkg.(type) {
	case *apipb.CipdPackage:
		return pkg.GetPackageName(), nil
	case *configpb.TaskTemplate_CipdPackage:
		return pkg.GetPkg(), nil
	default:
		return "", errors.Reason("unexpected type %T", pkg).Err()
	}
}

func validatePinnedInstanceVersion(v string) error {
	if common.ValidateInstanceID(v, common.AnyHash) == nil ||
		common.ValidateInstanceTag(v) == nil {
		return nil
	}
	return errors.Reason("%q is not a pinned instance version", v).Err()
}

// Tag checks a "<key>:<value>" tag is correct.
func Tag(tag string) error {
	parts := strings.Split(tag, ":")
	if len(parts) != 2 {
		return errors.Reason("tag must be in key:value form, not %q", tag).Err()
	}

	key, value := parts[0], parts[1]
	if err := keyLength(key); err != nil {
		return err
	}
	if err := DimensionValue(value); err != nil {
		return err
	}
	if reservedTags.Has(key) {
		return errors.Reason("tag %q is reserved for internal use and can't be assigned manually", key).Err()
	}
	return nil
}

// ServiceAccount checks a service account is correct.
func ServiceAccount(sa string) error {
	if len(sa) > maxServiceAccountLength {
		return errors.Reason("too long %q: %d > %d", sa, len(sa), maxServiceAccountLength).Err()
	}

	if _, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, sa)); err != nil {
		return errors.Reason("invalid %q: must be an email", sa).Err()
	}
	return nil
}

// Priority checks a priority is correct.
func Priority(p int32) error {
	if p < 0 || p > maxPriority {
		return errors.Reason("invalid %d, must be between 0 and %d", p, maxPriority).Err()
	}
	return nil
}

// BotPingTolerance checks a bot_ping_tolerance is correct.
func BotPingTolerance(bpt int64) error {
	if bpt < minBotPingTolanceSecs || bpt > maxBotPingTolanceSecs {
		return errors.Reason("invalid %d, must be between %d and %d", bpt, minBotPingTolanceSecs, maxBotPingTolanceSecs).Err()
	}
	return nil
}

// SecureURL checks a URL is valid and secure, except for localhost.
func SecureURL(u string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return err
	}
	if parsed.Hostname() == "" {
		return errors.Reason("invalid URL %q", u).Err()
	}

	localHosts := []string{"localhost", "127.0.0.1", "::1"}
	if slices.Contains(localHosts, parsed.Hostname()) {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.Reason("%q is not secure", u).Err()
		}
	} else if parsed.Scheme != "https" {
		return errors.Reason("%q is not secure", u).Err()
	}
	return nil
}

// PubSubTopicName validates the format of topic, extract the cloud project and topic id, and return them.
func PubSubTopicName(topic string) (string, string, error) {
	if topic == "" {
		return "", "", nil
	}
	if len(topic) > maxPubsubTopicLength {
		return "", "", errors.Reason("too long %s: %d > %d", topic, len(topic), maxPubsubTopicLength).Err()
	}

	matches := topicNameRE.FindAllStringSubmatch(topic, -1)
	if matches == nil || len(matches[0]) != 3 {
		return "", "", errors.Reason("topic %q does not match %q", topic, topicNameRE).Err()
	}

	cloudProj := matches[0][1]
	topicID := matches[0][2]
	// Only internal App Engine projects start with "google.com:", all other
	// project ids conform to cloudProjectIDRE.
	if !strings.HasPrefix(cloudProj, "google.com:") && !cloudProjectIDRE.MatchString(cloudProj) {
		return "", "", errors.Reason("cloud project id %q does not match %q", cloudProj, cloudProjectIDRE).Err()
	}
	if strings.HasPrefix(topicID, "goog") {
		return "", "", errors.Reason("topic id %q shouldn't begin with the string goog", topicID).Err()
	}
	if !topicIDRE.MatchString(topicID) {
		return "", "", errors.Reason("topic id %q does not match %q", topicID, topicIDRE).Err()
	}
	return cloudProj, topicID, nil
}

// teeErr saves `err` in `keep` and then returns `err`
func teeErr(err error, keep *error) error {
	*keep = err
	return err
}

// Path validates a path.
func Path(p string, maxLen int) error {
	var err error
	switch {
	case p == "":
		return errors.New("cannot be empty")
	case teeErr(Length(p, maxLen), &err) != nil:
		return err
	case strings.Contains(p, "\\"):
		return errors.New(`cannot contain "\\". On Windows forward-slashes will be replaced with back-slashes.`)
	case strings.HasPrefix(p, "/"):
		return errors.New(`cannot start with "/"`)
	case p != path.Clean(p):
		return errors.Reason("%q is not normalized. Normalized is %q", p, path.Clean(p)).Err()
	default:
		return nil
	}
}

// Length checks the value deos not exceed the limit.
func Length(val string, limit int) error {
	if len(val) > limit {
		return errors.Reason("too long %q: %d > %d", val, len(val), limit).Err()
	}
	return nil
}

type cacheEntry interface {
	GetName() string
	GetPath() string
}

// Caches validates a slice of cacheEntry.
//
// Also returns a set of cache paths.
//
// Can be used to validate
// * []*apipb.CacheEntry
// * []*configpb.TaskTemplate_CacheEntry
func Caches[C cacheEntry](caches []C) (pathSet stringset.Set, merr errors.MultiError) {
	if len(caches) > maxCacheCount {
		merr.MaybeAdd(errors.Reason("can have up to %d caches", maxCacheCount).Err())
	}
	nameSet := stringset.New(len(caches))
	pathSet = stringset.New(len(caches))
	for i, c := range caches {
		if !nameSet.Add(c.GetName()) {
			merr.MaybeAdd(errors.New("same cache name cannot be specified twice"))
		}
		if !pathSet.Add(c.GetPath()) {
			merr.MaybeAdd(errors.New("same cache path cannot be specified twice"))
		}
		if err := validateCacheName(c.GetName()); err != nil {
			merr.MaybeAdd(errors.Annotate(err, "cache name %d", i).Err())
		}
		if err := Path(c.GetPath(), maxCachePathLength); err != nil {
			merr.MaybeAdd(errors.Annotate(err, "cache path %d", i).Err())
		}
	}

	if merr.AsError() == nil {
		return pathSet, nil
	}
	return nil, merr
}

func validateCacheName(name string) error {
	if name == "" {
		return errors.New("required")
	}
	if err := Length(name, maxCacheNameLength); err != nil {
		return err
	}
	if !cacheNameRe.MatchString(name) {
		return errors.Reason("%q should match %s", name, cacheNameRe).Err()
	}
	return nil
}

// EnvVar validates an envvar name.
func EnvVar(ev string) error {
	var err error
	switch {
	case ev == "":
		return errors.New("required")
	case teeErr(Length(ev, maxEnvVarLength), &err) != nil:
		return err
	case !envVarRe.MatchString(ev):
		return errors.Reason("%q should match %s", ev, envVarRe).Err()
	default:
		return nil
	}
}
