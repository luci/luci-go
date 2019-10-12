// Copyright 2019 The LUCI Authors.
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

package main

import (
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateIncludeRequest returns a non-nil error if req is determined
// to be invalid.
func validateIncludeRequest(req *pb.IncludeRequest) error {
	if _, err := pbutil.ParseInvocationName(req.IncludingInvocation); err != nil {
		return errors.Annotate(err, "including_invocation").Err()
	}

	if _, err := pbutil.ParseInvocationName(req.IncludedInvocation); err != nil {
		return errors.Annotate(err, "included_invocation").Err()
	}

	if req.IncludedInvocation == req.IncludingInvocation {
		return errors.Reason("cannot include itself").Err()
	}

	if req.OverrideInvocation != "" {
		if _, err := pbutil.ParseInvocationName(req.OverrideInvocation); err != nil {
			return errors.Annotate(err, "override_invocation").Err()
		}
		if req.OverrideInvocation == req.IncludedInvocation {
			return errors.Reason("cannot override itself").Err()
		}
		if req.OverrideInvocation == req.IncludingInvocation {
			return errors.Reason("cannot include itself").Err()
		}
	}

	return nil
}
