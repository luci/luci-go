// Copyright 2015 The LUCI Authors.
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
	"github.com/luci/luci-go/common/retry/transient"
	"google.golang.org/api/googleapi"
)

// IsTransientAPIError returns true if error from Google API client indicates
// an error that can go away on its own in the future.
func IsTransientAPIError(err error) bool {
	if err == nil {
		return false
	}
	apiErr, _ := err.(*googleapi.Error)
	if apiErr == nil {
		return true // failed to get HTTP code => connectivity error => transient
	}
	return apiErr.Code >= 500 || apiErr.Code == 429 || apiErr.Code == 0
}

// WrapAPIError wraps error from Google API client in transient wrapper if
// necessary.
func WrapAPIError(err error) error {
	if IsTransientAPIError(err) {
		return transient.Tag.Apply(err)
	}
	return err
}
