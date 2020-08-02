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

package internals

import (
	"strings"

	"go.chromium.org/luci/common/errors"
)

// ValidateQueueName does quick check for some necessary conditions.
// It doesn't guarantee that queue is actually valid and exists.
func ValidateQueueName(q string) error {
	switch qs := strings.Split(q, "/"); {
	case q == "":
		return errors.New("queue name not given")
	case len(qs) != 6 || qs[0] != "projects" || qs[2] != "locations" || qs[4] != "queues":
		return errors.Reason("queue %q must be in format 'projects/PROJECT_ID/locations/LOCATION_ID/queues/QUEUE_ID'", q).Err()
	}
	return nil
}
