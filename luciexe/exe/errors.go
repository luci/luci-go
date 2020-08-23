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

package exe

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"google.golang.org/protobuf/proto"
)

// ErrBuildDetached is returned from the modification methods in this package
// when the Build has been detached from the context.
var ErrBuildDetached = errors.New("build is Detach'd")

// ErrStepClosed is returned from step modification methods when the step has
// been closed (i.e. the callback to `WithStep` has returned).
var ErrStepClosed = errors.New("step is closed")

// DuplicateLogTag is set on errors returned by the Log* methods of Build and
// Step when the caller requested a duplicate log name.
var DuplicateLogTag = errors.BoolTag{Key: errors.NewTagKey("duplicate log on build or step")}

type statusTagImpl struct{ Key errors.TagKey }

func (s statusTagImpl) With(value bbpb.Status) errors.TagValue {
	return errors.TagValue{Key: s.Key, Value: value}
}
func (s statusTagImpl) In(err error) (v bbpb.Status, ok bool) {
	d, ok := errors.TagValueIn(s.Key, err)
	if ok {
		v = d.(bbpb.Status)
	}
	return
}

// statusTag allows you to tag errors with a buildbucket status code.
var statusTag = statusTagImpl{errors.NewTagKey("has a bbpb.Status")}

// These values are 'error tags' which you can associate with a returned error:
//   errors.New("this was bad", StatusInfraFailure)
//   errors.Reason("reason: %s", explanation).Tag(StatusCanceled).Err()
//   errors.Annotate(err, "err actually OK").Tag(StatusSuccess).Err()
//
// See GetErrorStatus for the interpretation of these tags.
var (
	StatusSuccess      = statusTag.With(bbpb.Status_SUCCESS)
	StatusFailure      = statusTag.With(bbpb.Status_FAILURE)
	StatusInfraFailure = statusTag.With(bbpb.Status_INFRA_FAILURE)
	StatusCanceled     = statusTag.With(bbpb.Status_CANCELED)
)

// These values allow your error to set the corresponding fields in the the
// Build.StatuDetails object.
//
// See GetErrorStatus for the interpretation of these tags.
var (
	StatusDetailTimeout            = errors.BoolTag{Key: errors.NewTagKey("indicates a Timeout event")}
	StatusDetailResourceExhaustion = errors.BoolTag{Key: errors.NewTagKey("indicates a ResourceExhaustion event")}
)

// GetErrorStatus extracts the buildbucket Status and StatusDetails fields from
// an error.
//
// nil errors are SUCCESS with no details.
//
// If the error is tagged with Status*, then that's the status.
// If the error is tagged with StatusDetail*, then that's the status details.
//
// If no status is explicitly set and the error contains context.Canceled or
// context.DeadlineExceeded, then the status is CANCELED. For DeadlineExceeded,
// the details also have the Timeout field set.
//
// All untagged non-context errors have a status of FAILURE.
func GetErrorStatus(err error) (status bbpb.Status, details *bbpb.StatusDetails) {
	if err == nil {
		return bbpb.Status_SUCCESS, nil
	}

	mergeDetails := func(d *bbpb.StatusDetails) {
		if details == nil {
			details = d
		} else {
			proto.Merge(details, d)
		}
	}

	status = bbpb.Status_FAILURE

	switch {
	case errors.Contains(err, context.Canceled):
		status = bbpb.Status_CANCELED

	case errors.Contains(err, context.DeadlineExceeded):
		status = bbpb.Status_CANCELED
		mergeDetails(&bbpb.StatusDetails{
			Timeout: &bbpb.StatusDetails_Timeout{},
		})
	}

	if StatusDetailTimeout.In(err) {
		mergeDetails(&bbpb.StatusDetails{
			Timeout: &bbpb.StatusDetails_Timeout{},
		})
	}

	if StatusDetailResourceExhaustion.In(err) {
		mergeDetails(&bbpb.StatusDetails{
			ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
		})
	}

	if taggedStatus, ok := statusTag.In(err); ok {
		status = taggedStatus
	}
	return
}
