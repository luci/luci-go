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
)

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
//   errors.Annotate(err, "context").Tag(StatusSuccess).Err()
//
// Note that, by default, WithStep:
//   * will mark the step as SUCCESS if the callback returns nil
//   * will mark the step as FAILURE if the callback returns a non-nil (but
//     untagged) error.
//   * will mark the step as CANCELED if the callback returns a context
//     cancelation error.
var (
	StatusSuccess      = statusTag.With(bbpb.Status_SUCCESS)
	StatusFailure      = statusTag.With(bbpb.Status_FAILURE)
	StatusInfraFailure = statusTag.With(bbpb.Status_INFRA_FAILURE)
	StatusCanceled     = statusTag.With(bbpb.Status_CANCELED)
)

// These values allow your error to set the corresponding fields in the the
// Build.StatuDetails object.
var (
	StatusDetailTimeout            = errors.BoolTag{Key: errors.NewTagKey("indicates a Timeout event")}
	StatusDetailResourceExhaustion = errors.BoolTag{Key: errors.NewTagKey("indicates a ResourceExhaustion event")}
)

func getErrorStatus(err error) (status bbpb.Status, details *bbpb.StatusDetails) {
	switch {
	case err == nil:
		return bbpb.Status_SUCCESS, nil

	case err == context.Canceled:
		return bbpb.Status_CANCELED, nil

	case err == context.DeadlineExceeded:
		return bbpb.Status_CANCELED, &bbpb.StatusDetails{
			Timeout: &bbpb.StatusDetails_Timeout{},
		}

	default:
		timeout := StatusDetailTimeout.In(err)
		exhaust := StatusDetailResourceExhaustion.In(err)
		if timeout || exhaust {
			details = &bbpb.StatusDetails{}
			if timeout {
				details.Timeout = &bbpb.StatusDetails_Timeout{}
			}
			if exhaust {
				details.ResourceExhaustion = &bbpb.StatusDetails_ResourceExhaustion{}
			}
		}

		var ok bool
		if status, ok = statusTag.In(err); ok {
			return
		}
		return bbpb.Status_FAILURE, details
	}
}
