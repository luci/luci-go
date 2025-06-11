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

package pagination

import (
	"go.chromium.org/luci/common/errors"
)

type PageSizeLimiter struct {
	Max     int32
	Default int32
}

// Adjust the requested pageSize according to PageSizeLimiter.Max and
// PageSizeLimiter.Default as necessary.
func (psl *PageSizeLimiter) Adjust(pageSize int32) int32 {
	switch {
	case pageSize >= psl.Max:
		return psl.Max
	case pageSize > 0:
		return pageSize
	default:
		return psl.Default
	}
}

// ValidatePageSize returns a non-nil error if pageSize is invalid.
// Returns nil if pageSize is 0.
func ValidatePageSize(pageSize int32) error {
	if pageSize < 0 {
		return errors.New("negative")
	}
	return nil
}
