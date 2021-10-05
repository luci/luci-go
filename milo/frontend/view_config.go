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

package frontend

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/common"
)

// UpdateConfigHandler is an HTTP handler that handles configuration update
// requests.
func UpdateConfigHandler(c context.Context) error {
	projErr := common.UpdateProjects(c)
	if projErr != nil {
		if merr, ok := projErr.(errors.MultiError); ok {
			for _, ierr := range merr {
				logging.WithError(ierr).Errorf(c, "project update handler encountered error")
			}
		} else {
			logging.WithError(projErr).Errorf(c, "project update handler encountered error")
		}
	}
	_, servErr := common.UpdateServiceConfig(c)
	if servErr != nil {
		logging.WithError(servErr).Errorf(c, "service update handler encountered error")
	}

	return errors.Flatten(errors.NewMultiError(projErr, servErr))
}
