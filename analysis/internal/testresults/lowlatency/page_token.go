// Copyright 2026 The LUCI Authors.
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

package lowlatency

import (
	"fmt"
	"strconv"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/pagination"
)

// PageToken is a page token for a query. It represents the last row retrieved.
type PageToken struct {
	// The last source position returned.
	LastSourcePosition int64
}

// ParsePageToken parses a page token into a PageToken.
func ParsePageToken(token string) (PageToken, error) {
	if token == "" {
		// If provided to a request, indicates first page.
		// If returned in a response, indicates there are no more results.
		return PageToken{}, nil
	}
	parts, err := pagination.ParseToken(token)
	if err != nil {
		return PageToken{}, errors.Fmt("parse: %w", err)
	}
	const expectedParts = 1
	if len(parts) != expectedParts {
		return PageToken{}, errors.Fmt("expected %v components, got %d", expectedParts, len(parts))
	}
	pos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return PageToken{}, errors.Fmt("got non-integer last source position: %q", parts[0])
	}

	return PageToken{
		LastSourcePosition: pos,
	}, nil
}

// Serialize serializes a PageToken into a page token ready
// for an RPC response.
func (t PageToken) Serialize() string {
	if t == (PageToken{}) {
		// If provided to a request, indicates first page.
		// If returned in a response, indicates there are no more results.
		return ""
	}
	return pagination.Token(
		fmt.Sprintf("%d", t.LastSourcePosition),
	)
}
