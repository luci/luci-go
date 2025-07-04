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
	"maps"
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
	"go.chromium.org/luci/swarming/server/directoryocclusion"
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

	maxCIPDServerLength  = 1024
	maxCIPDPackageCount  = 64
	MaxPackagePathLength = 256

	maxEnvVarLength   = 64
	MaxEnvValueLength = 1024
	MaxEnvVarCount    = 64
)

var (
	dimensionKeyRe    = regexp.MustCompile(`^[a-zA-Z\-\_\.][0-9a-zA-Z\-\_\.]*$`)
	sessionIDRe       = regexp.MustCompile(`^[a-z0-9\-_/]{1,50}$`)
	botRequestUUDIDRe = regexp.MustCompile(`^[a-zA-Z0-9\-_/]{0,50}$`)
	reservedTags      = stringset.NewFromSlice([]string{"swarming.terminate"}...)

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
		return errors.Fmt("the key should match %s", dimensionKeyRe)
	}
	return nil
}

func keyLength(key string) error {
	if key == "" {
		return errors.New("the key cannot be empty")
	}
	if len(key) > maxDimensionKeyLen {
		return errors.Fmt("the key should be no longer than %d (got %d)", maxDimensionKeyLen, len(key))
	}
	return nil
}

func tagOrDimValue(val string) error {
	if len(val) > maxDimensionValLen {
		return errors.Fmt("the value should be no longer than %d (got %d)", maxDimensionValLen, len(val))
	}
	if strings.TrimSpace(val) != val {
		return errors.New("the value should have no leading or trailing spaces")
	}
	return nil
}

// DimensionValue checks if `val` can be a dimension value for the given key.
func DimensionValue(key, val string) error {
	if val == "" {
		return errors.New("the value cannot be empty")
	}
	if err := tagOrDimValue(val); err != nil {
		return err
	}
	switch key {
	case "id":
		return validateBotID(val)
	case "pool":
		return validatePool(val)
	default:
		return nil
	}
}

// validateBotID are extra validation rules for "id" dimension values.
func validateBotID(val string) error {
	if strings.Contains(val, ":") {
		return errors.New(`bot ID is not allowed to contain ":"`)
	}
	return nil
}

// validatePool are extra validation rules for "pool" dimension values.
func validatePool(val string) error {
	if strings.Contains(val, ":") {
		return errors.New(`pool ID is not allowed to contain ":"`)
	}
	return nil
}

// SessionID checks if `val` is a valid bot session ID.
func SessionID(val string) error {
	if !sessionIDRe.MatchString(val) {
		return errors.Fmt("should match %s", sessionIDRe)
	}
	return nil
}

// BotRequestUUID checks if `val` is a valid bot request UUID.
//
// An empty string is considered valid as well.
func BotRequestUUID(val string) error {
	if !botRequestUUDIDRe.MatchString(val) {
		return errors.Fmt("should match %s", botRequestUUDIDRe)
	}
	return nil
}

// CIPDServer checks CIPD server is correct.
func CIPDServer(server string) error {
	if server == "" {
		return errors.New("required")
	}
	if err := Length(server, maxCIPDServerLength); err != nil {
		return err
	}
	return SecureURL(server)
}

// CIPDPackageName checks CIPD package name is correct.
//
// If allowTemplate is true, the package name is allowed to be a template e.g.
// have "${platform}" and other substitutions inside.
func CIPDPackageName(pkg string, allowTemplate bool) error {
	if pkg == "" {
		return errors.New("required")
	}

	expanded, err := cipdExpander.Expand(pkg)
	if err != nil {
		return errors.Fmt("bad package name template %q: %w", pkg, err)
	}
	if !allowTemplate && pkg != expanded {
		return errors.Fmt("package name template %q is not allowed here", pkg)
	}

	// No need to annotate the error, it has all details already.
	return common.ValidatePackageName(expanded)
}

// CIPDPackageVersion checks CIPD package version is correct.
func CIPDPackageVersion(ver string) error {
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
func CIPDPackages[P cipdPkg](packages []P, requirePinnedVer bool, doc *directoryocclusion.Checker, pkgSource string) errors.MultiError {
	var merr errors.MultiError
	if len(packages) > maxCIPDPackageCount {
		merr.MaybeAdd(errors.Fmt("can have up to %d packages", maxCIPDPackageCount))
	}

	type pathName struct {
		p    string
		name string
	}
	pkgPathNames := make(map[pathName]struct{}, len(packages))
	for i, pkg := range packages {
		pkgName, err := packageName(pkg)
		if err != nil {
			merr.MaybeAdd(errors.Fmt("name: %w", err))
		}

		subMerr := validateCIPDPackage(pkgName, pkg, requirePinnedVer, doc, pkgSource)
		if subMerr.AsError() != nil {
			for _, err := range subMerr.Unwrap() {
				merr.MaybeAdd(errors.WrapIf(err, "package %d (%s)", i, pkgName))
			}
		}

		pn := pathName{pkg.GetPath(), pkgName}
		if _, ok := pkgPathNames[pn]; ok {
			merr.MaybeAdd(errors.Fmt("package %q is specified more than once in path %s", pkgName, pkg.GetPath()))
		} else {
			pkgPathNames[pn] = struct{}{}
		}
	}

	docErrs := doc.Conflicts()
	for _, err := range docErrs {
		merr.MaybeAdd(err)
	}

	return merr
}

func validateCIPDPackage(pkgName string, pkg cipdPkg, requirePinnedVer bool, doc *directoryocclusion.Checker, pkgSource string) errors.MultiError {
	var merr errors.MultiError

	if err := CIPDPackageName(pkgName, true); err != nil {
		merr.MaybeAdd(errors.Fmt("name: %w", err))
	}

	if err := CIPDPackageVersion(pkg.GetVersion()); err != nil {
		merr.MaybeAdd(errors.Fmt("version: %w", err))
	}

	if err := Path(pkg.GetPath(), MaxPackagePathLength, false); err != nil {
		merr.MaybeAdd(errors.Fmt("path: %w", err))
	}

	// All cipd packages are considered compatible in terms of paths: it's
	// totally legit to install many packages in the same directory.
	// Thus we set the owner for all cipd packages to pkgSource.
	doc.Add(pkg.GetPath(), pkgSource, fmt.Sprintf("%s:%s", pkgName, pkg.GetVersion()))

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
		return "", errors.Fmt("unexpected type %T", pkg)
	}
}

func validatePinnedInstanceVersion(v string) error {
	if common.ValidateInstanceID(v, common.AnyHash) == nil ||
		common.ValidateInstanceTag(v) == nil {
		return nil
	}
	return errors.Fmt("%q is not a pinned instance version", v)
}

// Tag checks a "<key>:<value>" tag is correct.
func Tag(tag string) error {
	parts := strings.SplitN(tag, ":", 2)
	if len(parts) != 2 {
		return errors.Fmt("tag must be in key:value form, not %q", tag)
	}

	key, value := parts[0], parts[1]
	if err := keyLength(key); err != nil {
		return err
	}

	if err := tagOrDimValue(value); err != nil {
		return err
	}
	if reservedTags.Has(key) {
		return errors.Fmt("tag %q is reserved for internal use and can't be assigned manually", key)
	}
	return nil
}

// ServiceAccount checks a service account is correct.
func ServiceAccount(sa string) error {
	if len(sa) > maxServiceAccountLength {
		return errors.Fmt("too long %q: %d > %d", sa, len(sa), maxServiceAccountLength)
	}

	if _, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, sa)); err != nil {
		return errors.Fmt("invalid %q: must be an email", sa)
	}
	return nil
}

// Priority checks a priority is correct.
func Priority(p int32) error {
	// Priority 0 is reserved for terminate tasks.
	if p < 1 || p > maxPriority {
		return errors.Fmt("invalid %d, must be between 1 and %d", p, maxPriority)
	}
	return nil
}

// BotPingTolerance checks a bot_ping_tolerance is correct.
func BotPingTolerance(bpt int64) error {
	if bpt < minBotPingTolanceSecs || bpt > maxBotPingTolanceSecs {
		return errors.Fmt("invalid %d, must be between %d and %d", bpt, minBotPingTolanceSecs, maxBotPingTolanceSecs)
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
		return errors.Fmt("invalid URL %q", u)
	}

	localHosts := []string{"localhost", "127.0.0.1", "::1"}
	if slices.Contains(localHosts, parsed.Hostname()) {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.Fmt("%q is not secure", u)
		}
	} else if parsed.Scheme != "https" {
		return errors.Fmt("%q is not secure", u)
	}
	return nil
}

// PubSubTopicName validates the format of topic, extract the cloud project and topic id, and return them.
func PubSubTopicName(topic string) (string, string, error) {
	if topic == "" {
		return "", "", nil
	}
	if len(topic) > maxPubsubTopicLength {
		return "", "", errors.Fmt("too long %s: %d > %d", topic, len(topic), maxPubsubTopicLength)
	}

	matches := topicNameRE.FindAllStringSubmatch(topic, -1)
	if matches == nil || len(matches[0]) != 3 {
		return "", "", errors.Fmt("topic %q does not match %q", topic, topicNameRE)
	}

	cloudProj := matches[0][1]
	topicID := matches[0][2]
	// Only internal App Engine projects start with "google.com:", all other
	// project ids conform to cloudProjectIDRE.
	if !strings.HasPrefix(cloudProj, "google.com:") && !cloudProjectIDRE.MatchString(cloudProj) {
		return "", "", errors.Fmt("cloud project id %q does not match %q", cloudProj, cloudProjectIDRE)
	}
	if strings.HasPrefix(topicID, "goog") {
		return "", "", errors.Fmt("topic id %q shouldn't begin with the string goog", topicID)
	}
	if !topicIDRE.MatchString(topicID) {
		return "", "", errors.Fmt("topic id %q does not match %q", topicID, topicIDRE)
	}
	return cloudProj, topicID, nil
}

// teeErr saves `err` in `keep` and then returns `err`
func teeErr(err error, keep *error) error {
	*keep = err
	return err
}

// Path validates a path.
func Path(p string, maxLen int, allowWinPath bool) error {
	var err error
	switch {
	case p == "":
		return errors.New("cannot be empty")
	case teeErr(Length(p, maxLen), &err) != nil:
		return err
	case !allowWinPath && strings.Contains(p, "\\"):
		return errors.New(`cannot contain "\\". On Windows forward-slashes will be replaced with back-slashes.`)
	case strings.HasPrefix(p, "/"):
		return errors.New(`cannot start with "/"`)
	case p != path.Clean(p):
		return errors.Fmt("%q is not normalized. Normalized is %q", p, path.Clean(p))
	default:
		return nil
	}
}

// Length checks the value does not exceed the limit.
func Length(val string, limit int) error {
	if len(val) > limit {
		return errors.Fmt("too long %q: %d > %d", val, len(val), limit)
	}
	return nil
}

type cacheEntry interface {
	GetName() string
	GetPath() string
}

// Caches validates a slice of cacheEntry.
//
// Also returns a directoryoccusion.Checker with all of the cache pathes.
//
// Can be used to validate
// * []*apipb.CacheEntry
// * []*configpb.TaskTemplate_CacheEntry
func Caches[C cacheEntry](caches []C, cacheSource string) (*directoryocclusion.Checker, errors.MultiError) {
	var merr errors.MultiError
	if len(caches) > maxCacheCount {
		merr.MaybeAdd(errors.Fmt("can have up to %d caches", maxCacheCount))
	}

	doc := directoryocclusion.NewChecker("")
	nameSet := stringset.New(len(caches))
	for i, c := range caches {
		if !nameSet.Add(c.GetName()) {
			merr.MaybeAdd(errors.New("same cache name cannot be specified twice"))
		}
		if err := Path(c.GetPath(), maxCachePathLength, false); err != nil {
			merr.MaybeAdd(errors.Fmt("cache path %d: %w", i, err))
			continue
		}
		// Caches are all unique; they can't overlap.
		doc.Add(c.GetPath(), fmt.Sprintf("%s:%s", cacheSource, c.GetName()), "")
		if err := validateCacheName(c.GetName()); err != nil {
			merr.MaybeAdd(errors.Fmt("cache name %d: %w", i, err))
		}
	}
	docErrs := doc.Conflicts()
	for _, err := range docErrs {
		merr.MaybeAdd(err)
	}

	return doc, merr
}

func validateCacheName(name string) error {
	if name == "" {
		return errors.New("required")
	}
	if err := Length(name, maxCacheNameLength); err != nil {
		return err
	}
	if !cacheNameRe.MatchString(name) {
		return errors.Fmt("%q should match %s", name, cacheNameRe)
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
		return errors.Fmt("%q should match %s", ev, envVarRe)
	default:
		return nil
	}
}

// BotDimensions validates dimensions as self-reported by a bot.
//
// Among other checks, verifies the bot reports "id" dimension with a single
// value that will be used as the bot ID.
func BotDimensions(dims map[string][]string) errors.MultiError {
	var perKeyErrs map[string]errors.MultiError

	recordErr := func(key string, err error) {
		if perKeyErrs == nil {
			perKeyErrs = make(map[string]errors.MultiError, 1)
		}
		perKeyErrs[key] = append(perKeyErrs[key], err)
	}

	seenID := false
	for key, vals := range dims {
		if err := DimensionKey(key); err != nil {
			recordErr(key, err)
		}
		if len(vals) == 0 {
			recordErr(key, errors.New("values list should not be empty"))
		}
		seen := stringset.New(len(vals))
		for _, val := range vals {
			if err := DimensionValue(key, val); err != nil {
				recordErr(key, errors.Fmt("bad value %q: %w", trimLen(val, maxDimensionValLen+5), err))
			} else if !seen.Add(val) {
				recordErr(key, errors.Fmt("duplicate value %q", val))
			}
		}
		if key == "id" {
			seenID = true
			if len(vals) > 1 {
				recordErr(key, errors.New("must have only one value"))
			}
		}
	}

	if !seenID {
		recordErr("id", errors.New("a value is missing"))
	}

	if len(perKeyErrs) == 0 {
		return nil
	}

	// Make sure errors are reported in a deterministic order.
	var merr errors.MultiError
	for _, key := range slices.Sorted(maps.Keys(perKeyErrs)) {
		for _, err := range perKeyErrs[key] {
			merr = append(merr, errors.WrapIf(err, "key %q", trimLen(key, maxDimensionKeyLen+5)))
		}
	}

	return merr
}

// trimLen cuts the string to make sure it is no longer than the given length.
func trimLen(val string, max int) string {
	if len(val) <= max {
		return val
	}
	return val[:max-3] + "..."
}
