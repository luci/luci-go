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
	"time"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateCreateInvocationRequest returns an error if req is determined to be
// invalid.
func validateCreateInvocationRequest(req *pb.CreateInvocationRequest, now time.Time) error {
	if err := pbutil.ValidateInvocationID(req.InvocationId); err != nil {
		return errors.Annotate(err, "invocation_id").Err()
	}

	inv := req.Invocation
	if inv == nil {
		return nil
	}

	if err := pbutil.ValidateStringPairs(inv.GetTags()); err != nil {
		return errors.Annotate(err, "invocation.tags").Err()
	}

	if inv.GetDeadline() != nil {
		switch deadline, err := ptypes.Timestamp(inv.Deadline); {
		case err != nil:
			return err
		case deadline.Sub(now) < 10*time.Second:
			return errors.Reason("invocation.deadline must be at least 10 seconds in the future").Err()
		case deadline.Sub(now) > 2*24*time.Hour:
			return errors.Reason("invocation.deadline must be before 48h in the future").Err()
		}
	}

	if inv.GetBaseTestVariantDef() != nil {
		if err := pbutil.ValidateVariantDef(inv.BaseTestVariantDef); err != nil {
			return errors.Annotate(err, "invocation.base_test_variant_def").Err()
		}
	}

	return nil
}
