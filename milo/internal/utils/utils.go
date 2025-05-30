// Copyright 2019 The LUCI Authors.
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

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
)

// MergeStrings merges multiple string slices together into a single slice,
// removing duplicates.
func MergeStrings(sss ...[]string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, ss := range sss {
		for _, s := range ss {
			if seen[s] {
				continue
			}
			seen[s] = true
			result = append(result, s)
		}
	}
	sort.Strings(result)
	return result
}

// ObfuscateEmail converts a string containing email address email@address.com
// into email<junk>@address.com.
func ObfuscateEmail(email string) template.HTML {
	email = template.HTMLEscapeString(email)
	return template.HTML(strings.Replace(
		email, "@", "<span style=\"display:none\">ohnoyoudont</span>@", -1))
}

// ShortenEmail shortens Google emails.
func ShortenEmail(email string) string {
	return strings.Replace(email, "@google.com", "", -1)
}

// TagGRPC annotates some gRPC with Milo specific semantics, specifically:
// * Marks the error as Unauthorized if the user is not logged in,
// and the underlying error was a 403 or 404.
// * Otherwise, tag the error with the original error code.
func TagGRPC(c context.Context, err error) error {
	if err == nil {
		return nil
	}
	loggedIn := auth.CurrentIdentity(c) != identity.AnonymousIdentity
	code := grpcutil.Code(err)
	if code == codes.NotFound || code == codes.PermissionDenied {
		// Mask the errors, so they look the same.
		if loggedIn {
			return grpcutil.NotFoundTag.Apply(errors.New("not found"))
		}
		return grpcutil.UnauthenticatedTag.Apply(errors.New("not logged in"))
	}
	return status.Error(code, err.Error())
}

// ParseIntFromForm parses an integer from a form.
func ParseIntFromForm(form url.Values, key string, base int, bitSize int) (int64, error) {
	input, err := ReadExactOneFromForm(form, key)
	if err != nil {
		return 0, err
	}
	ret, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return 0, errors.Fmt("invalid %v; expected an integer; actual value: %v: %w", key, input, err)
	}
	return ret, nil
}

// ReadExactOneFromForm read a string from a form.
// There must be exactly one and non-empty entry of the given key in the form.
func ReadExactOneFromForm(form url.Values, key string) (string, error) {
	input := form[key]
	if len(input) != 1 || input[0] == "" {
		return "", fmt.Errorf("multiple or missing %v; actual value: %v", key, input)
	}
	return input[0], nil
}

// BucketResourceID returns a string identifying the bucket resource.
// It is used when checking bucket permission.
func BucketResourceID(project, bucket string) string {
	return fmt.Sprintf("luci.%s.%s", project, bucket)
}

// LegacyBuilderIDString returns a legacy string identifying the builder.
// It is used in the Milo datastore.
func LegacyBuilderIDString(bid *buildbucketpb.BuilderID) string {
	return fmt.Sprintf("buildbucket/%s/%s", BucketResourceID(bid.Project, bid.Bucket), bid.Builder)
}

var ErrInvalidLegacyBuilderID = errors.New("the string is not a valid legacy builder ID (format: buildbucket/luci.<project>.<bucket>/<builder>)")
var legacyBuilderIDRe = regexp.MustCompile(`^buildbucket/luci\.([^./]+)\.([^/]+)/([^/]+)$`)

// ParseLegacyBuilderID parses the legacy builder ID
// (e.g. `buildbucket/luci.<project>.<bucket>/<builder>`) and returns the
// BuilderID struct.
func ParseLegacyBuilderID(bid string) (*buildbucketpb.BuilderID, error) {
	match := legacyBuilderIDRe.FindStringSubmatch(bid)
	if len(match) == 0 {
		return nil, ErrInvalidLegacyBuilderID
	}
	return &buildbucketpb.BuilderID{
		Project: match[1],
		Bucket:  match[2],
		Builder: match[3],
	}, nil
}

var ErrInvalidBuilderID = errors.New("the string is not a valid builder ID")
var builderIDRe = regexp.MustCompile("^([^/]+)/([^/]+)/([^/]+)$")

// ParseBuilderID parses the canonical builder ID
// (e.g. `<project>/<bucket>/<builder>`) and returns the BuilderID struct.
func ParseBuilderID(bid string) (*buildbucketpb.BuilderID, error) {
	match := builderIDRe.FindStringSubmatch(bid)
	if len(match) == 0 {
		return nil, ErrInvalidBuilderID
	}
	return &buildbucketpb.BuilderID{
		Project: match[1],
		Bucket:  match[2],
		Builder: match[3],
	}, nil
}

var buildbucketBuildIDRe = regexp.MustCompile(`^buildbucket/(\d+)$`)
var legacyBuildbucketBuildIDRe = regexp.MustCompile(`^buildbucket/luci\.([^./]+)\.([^/]+)/([^/]+)/(\d+)$`)
var ErrInvalidLegacyBuildID = errors.New("the string is not a valid legacy build ID")

// ParseBuildbucketBuildID parses the legacy build ID in the format of
// `buildbucket/<build_id>`
func ParseBuildbucketBuildID(bid string) (buildID int64, err error) {
	match := buildbucketBuildIDRe.FindStringSubmatch(bid)
	if len(match) == 0 {
		return 0, ErrInvalidLegacyBuildID
	}
	buildID, err = strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, ErrInvalidBuilderID
	}
	return buildID, nil
}

// ParseLegacyBuildbucketBuildID parses the legacy build ID in the format of
// `buildbucket/luci.<project>.<bucket>/<builder>/<number>`.
func ParseLegacyBuildbucketBuildID(bid string) (builderID *buildbucketpb.BuilderID, number int32, err error) {
	match := legacyBuildbucketBuildIDRe.FindStringSubmatch(bid)
	if len(match) == 0 {
		return nil, 0, ErrInvalidLegacyBuildID
	}
	builderID = &buildbucketpb.BuilderID{
		Project: match[1],
		Bucket:  match[2],
		Builder: match[3],
	}
	buildNum, err := strconv.ParseInt(match[4], 10, 32)
	if err != nil {
		return nil, 0, ErrInvalidLegacyBuildID
	}
	return builderID, int32(buildNum), nil
}

// GetJSONData fetches data from the given URL, parses the response body to `out`.
// It follows redirection and returns an error if the status code is 4xx or 5xx.
func GetJSONData(client *http.Client, url string, out any) (err error) {
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Flatten(errors.NewMultiError(response.Body.Close(), err))
	}()

	if response.StatusCode >= 400 && response.StatusCode <= 599 {
		return fmt.Errorf("failed to fetch data: %q returned code %q", url, response.Status)
	}

	dec := json.NewDecoder(response.Body)
	return dec.Decode(out)
}

// ReplaceNSEWith takes an errors.MultiError returned by a datastore.Get() on a
// slice (which is always a MultiError), filters out all
// datastore.ErrNoSuchEntity or replaces it with replacement instances, and
// returns an error generated by errors.LazyMultiError.
func ReplaceNSEWith(err errors.MultiError, replacement error) error {
	lme := errors.NewLazyMultiError(len(err))
	for i, ierr := range err {
		if ierr == datastore.ErrNoSuchEntity {
			ierr = replacement
		}
		lme.Assign(i, ierr)
	}
	return lme.Get()
}
