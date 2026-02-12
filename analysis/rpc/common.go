// Copyright 2022 The LUCI Authors.
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

package rpc

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/pagination"
)

// PageSizeLimiter limits the page size for RPCs.
var PageSizeLimiter = pagination.PageSizeLimiter{
	Default: 100,
	Max:     1000,
}

// ValidateTestIDPart validates a part of a test ID (e.g. substring of a test ID).
func ValidateTestIDPart(testIDPart string) error {
	if testIDPart == "" {
		return errors.New("unspecified")
	}
	if len(testIDPart) > 512 {
		return errors.New("length exceeds 512 bytes")
	}
	if !utf8.ValidString(testIDPart) {
		return errors.New("not a valid utf8 string")
	}
	if !norm.NFC.IsNormalString(testIDPart) {
		return errors.New("not in unicode normalized form C")
	}
	for i, rune := range testIDPart {
		if !unicode.IsPrint(rune) {
			return fmt.Errorf("non-printable rune %+q at byte index %d", rune, i)
		}
	}
	return nil
}
