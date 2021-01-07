// Copyright 2021 The LUCI Authors.
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

package pbutil

import (
	"strings"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ValidateCommitPosition returns a non-nil error if commit is invalid.
// Partially copied from go/chromium.org/luci/buildbucket/appengine/rpc.validateCommit
func ValidateCommitPosition(cm *pb.CommitPosition) error {
	switch {
	case cm == nil:
		return errors.Reason("commit is required").Err()
	case cm.GetHost() == "":
		return errors.Reason("host is required").Err()
	case cm.GetProject() == "":
		return errors.Reason("project is required").Err()
	case cm.GetRef() == "" || !strings.HasPrefix(cm.Ref, "refs/"):
		return errors.Reason("ref must match refs/.*").Err()
	case cm.GetPosition() <= 0:
		return errors.Reason("position is required").Err()
	}
	return nil
}
