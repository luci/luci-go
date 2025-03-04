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

package build

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
)

type buildStatus struct {
	status  bbpb.Status
	details *bbpb.StatusDetails
}

var statusTag = errtag.Make("build Status", (*buildStatus)(nil))

// AttachStatus attaches a buildbucket status (and details) to a given error.
//
// If such an error is handled by Step.End or State.End, that step/build will
// have its status updated to match.
//
// AttachStatus allows overriding the attached status if the error already has
// one.
//
// This panics if the status is a non-terminal status like SCHEDULED or STARTED.
//
// This is a no-op if the error is nil.
func AttachStatus(err error, status bbpb.Status, details *bbpb.StatusDetails) error {
	if !protoutil.IsEnded(status) {
		panic(errors.Reason("AttachStatus cannot be used with non-terminal status %q", status).Err())
	}
	if err == nil {
		return nil
	}
	if details != nil {
		details = proto.Clone(details).(*bbpb.StatusDetails)
	}
	return statusTag.ApplyValue(err, &buildStatus{status, details})
}

// ExtractStatus retrieves the Buildbucket status (and details) from a given
// error.
//
// This returns:
//   - (SUCCESS, nil) on nil error
//   - Any values attached with AttachStatus.
//   - (CANCELED, nil) on context.Canceled
//   - (INFRA_FAILURE, &bbpb.StatusDetails{Timeout: {}}) on
//     context.DeadlineExceeded
//   - (FAILURE, nil) otherwise
//
// This function is used internally by Step.End and State.End, but is provided
// publically for completeness.
func ExtractStatus(err error) (bbpb.Status, *bbpb.StatusDetails) {
	if err == nil {
		return bbpb.Status_SUCCESS, nil
	}
	if bs, ok := statusTag.Value(err); ok {
		details := bs.details
		if details != nil {
			details = proto.Clone(details).(*bbpb.StatusDetails)
		}
		return bs.status, details
	}
	switch err {
	case context.Canceled:
		return bbpb.Status_CANCELED, nil
	case context.DeadlineExceeded:
		return bbpb.Status_INFRA_FAILURE, &bbpb.StatusDetails{
			Timeout: &bbpb.StatusDetails_Timeout{},
		}
	}
	return bbpb.Status_FAILURE, nil
}

func computePanicStatus(err error) (status bbpb.Status, message string) {
	if errors.IsPanicking(2) {
		message = "PANIC"
		// TODO(iannucci): include details of panic in SummaryMarkdown or log?
		// How to prevent panic dump from showing up at every single step on the
		// stack?
		status = bbpb.Status_INFRA_FAILURE
	} else {
		status, _ = ExtractStatus(err)
		if err != nil {
			message = err.Error()
		}
	}
	return
}

func logStatus(ctx context.Context, status bbpb.Status, message, markdown string) {
	logf := logging.Errorf
	switch status {
	case bbpb.Status_SUCCESS:
		logf = logging.Infof
	case bbpb.Status_CANCELED:
		logf = logging.Warningf
	}

	logMsg := fmt.Sprintf("set status: %s", status)
	if len(message) > 0 {
		logMsg += ": " + message
	}
	if len(markdown) > 0 {
		logMsg += "\n  with SummaryMarkdown:\n" + markdown
	}
	logf(ctx, "%s", logMsg)
}
