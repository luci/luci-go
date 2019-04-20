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
	"context"
	"sort"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
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
		} else {
			return errors.Reason("not logged in").Tag(grpcutil.UnauthenticatedTag).Err()
		}
	}
	return grpcutil.ToGRPCErr(err)
}
