// Copyright 2020 The LUCI Authors.
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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
)

// MostSevereError returns the most severer error in order of
// non-transient => transient => nil.
func MostSevereError(errs errors.MultiError) error {
	var firstTrans error
	for _, err := range errs {
		switch {
		case err == nil:
		case !transient.Tag.In(err):
			return err
		case firstTrans == nil:
			firstTrans = err
		}
	}
	return firstTrans
}
