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

package common

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
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
	loggedIn := auth.CurrentIdentity(c) != identity.AnonymousIdentity
	code := grpcutil.Code(err)
	if code == codes.NotFound || code == codes.PermissionDenied {
		// Mask the errors, so they look the same.
		if loggedIn {
			return errors.Reason("not found").Tag(grpcutil.NotFoundTag).Err()
		}
		return errors.Reason("not logged in").Tag(grpcutil.UnauthenticatedTag).Err()
	}
	return grpcutil.ToGRPCErr(err)
}

// ParseIntFromForm parses an integer from a form.
func ParseIntFromForm(form url.Values, key string, base int, bitSize int) (int64, error) {
	input, err := ReadExactOneFromForm(form, key)
	if err != nil {
		return 0, err
	}
	ret, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return 0, errors.Annotate(err, "invalid %v; expected an integer; actual value: %v", key, input).Err()
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

// LegacyBuilderIDString returns a legacy string identifying the builder.
// It is used in the Milo datastore.
func LegacyBuilderIDString(bid *buildbucketpb.BuilderID) string {
	return fmt.Sprintf("buildbucket/luci.%s.%s/%s", bid.Project, bid.Bucket, bid.Builder)
}

// JSONMarshalCompressed converts a message into compressed JSON form, suitable for storing in memcache.
func JSONMarshalCompressed(message interface{}) ([]byte, error) {
	// Compress using zlib.
	b := bytes.Buffer{}
	w := zlib.NewWriter(&b)
	enc := json.NewEncoder(w)

	if err := enc.Encode(message); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

// JSONUnmarshalCompressed converts a message back from compressed JSON form.
func JSONUnmarshalCompressed(serialized []byte, out interface{}) error {
	// Decompress using zlib.
	r, err := zlib.NewReader(bytes.NewReader(serialized))
	if err != nil {
		return err
	}

	dec := json.NewDecoder(r)
	return dec.Decode(out)
}

// GetJSONData fetches data from the given URL, parses the response body to `out`.
// It follows redirection and returns an error if the status code is 4xx or 5xx.
func GetJSONData(client *http.Client, url string, out interface{}) (err error) {
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
