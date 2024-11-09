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

package protoutil

import (
	bbprotoutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"

	milopb "go.chromium.org/luci/milo/proto/v1"
)

// ValidateConsolePredicate validates the given console predicate.
func ValidateConsolePredicate(predicate *milopb.ConsolePredicate) error {
	if predicate.GetBuilder() != nil {
		err := bbprotoutil.ValidateRequiredBuilderID(predicate.Builder)
		if err != nil {
			return errors.Annotate(err, "builder").Err()
		}
	}

	return nil
}
