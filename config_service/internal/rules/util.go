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

package rules

import (
	"net/mail"
	"net/url"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"
)

func validateAccess(vctx *validation.Context, access string) {
	switch {
	case access == "":
		vctx.Errorf("not specified")
	case strings.HasPrefix(access, "group:"):
		if group := access[len("group:"):]; !auth.IsValidGroupName(group) {
			vctx.Errorf("invalid auth group: %q", group)
		}
	case strings.Contains(access, ":"):
		if _, err := identity.MakeIdentity(access); err != nil {
			vctx.Errorf("invalid identity %q; reason: %s", access, err)
		}
	default:
		validateEmail(vctx, access)
	}
}

func validateEmail(vctx *validation.Context, email string) {
	switch {
	case email == "":
		vctx.Errorf("email not specified")
	default:
		if _, err := mail.ParseAddress(email); err != nil {
			vctx.Errorf("invalid email address: %s", err)
		}
	}
}

// validateSorted checks whether items in the slice are sorted by ID.
//
// The ID is derived by `getIDFn`. Empty ID is skipped during the check with
// the assumption that other check in this package will complain about empty
// id.
func validateSorted[T any](vctx *validation.Context, slice []T, listName string, getIDFn func(T) string) {
	var prevID string
	for _, element := range slice {
		curID := getIDFn(element)
		if curID == "" {
			continue // empty id would be caught by some other validation rules.
		}
		if prevID != "" && strings.Compare(prevID, curID) > 0 {
			vctx.Errorf("%s are not sorted by id. First offending id: %q", listName, curID)
			return
		}
		prevID = curID
	}
}

// validateUniqueID checks whether the given id is empty and whether it has been
// seen before. Also optionally applies additional checks defined by
// `validateIDFn`.
func validateUniqueID(vctx *validation.Context, id string, seen stringset.Set, validateIDFn func(vctx *validation.Context, id string)) {
	switch {
	case id == "":
		vctx.Errorf("not specified")
	case seen.Has(id):
		vctx.Errorf("duplicate: %q", id)
	default:
		seen.Add(id)
		if validateIDFn != nil {
			validateIDFn(vctx, id)
		}
	}
}

func validateURL(vctx *validation.Context, urlString string) {
	if urlString == "" {
		vctx.Errorf("not specified")
		return
	}
	u, err := url.Parse(urlString)
	if err != nil {
		vctx.Errorf("invalid url: %s", err)
		return
	}
	if u.Hostname() == "" {
		vctx.Errorf("hostname must be specified")
	}
	if u.Scheme != "https" {
		vctx.Errorf("scheme must be \"https\"")
	}
}
