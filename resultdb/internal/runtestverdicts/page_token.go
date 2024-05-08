// Copyright 2024 The LUCI Authors.
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

package runtestverdicts

import (
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/pagination"
)

// PageToken represents a pagination position in the query.
type PageToken struct {
	AfterTestID      string
	AfterVariantHash string
}

// ParsePageToken parses a page token string into a PageToken.
func ParsePageToken(token string) (PageToken, error) {
	if token == "" {
		return PageToken{}, nil
	}

	parts, err := pagination.ParseToken(token)
	if err != nil {
		return PageToken{}, err
	}
	if len(parts) != 2 {
		return PageToken{}, pagination.InvalidToken(errors.Reason("invalid number of components").Err())
	}
	return PageToken{
		AfterTestID:      parts[0],
		AfterVariantHash: parts[1],
	}, nil
}

// Serialize serializes the page token.
func (t PageToken) Serialize() string {
	if t.AfterTestID == "" && t.AfterVariantHash == "" {
		return ""
	}
	return pagination.Token(t.AfterTestID, t.AfterVariantHash)
}
