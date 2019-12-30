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

package appstatus

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
)

var appStatusTagKey = errors.NewTagKey("application-specific response status")

// Errf returns an error with an application-specific status.
// The message will be shared with the RPC client as is.
func Errf(code codes.Code, format string, args ...interface{}) error {
	st := status.Newf(code, format, args...)
	return Attach(st.Err(), st)
}

// Attach attaches an application-specific status to the error.
// The status will be shared to the RPC client as is.
// If err already has a status attached, panics.
func Attach(err error, status *status.Status) error {
	if status.Code() == codes.OK {
		panic("cannot attach an OK status")
	}

	if _, ok := FromError(err); ok {
		panic("err already has an application-specific status")
	}

	tag := errors.TagValue{
		Key:   appStatusTagKey,
		Value: status,
	}
	return errors.Annotate(err, "attaching a status").Tag(tag).Err()
}

// Attachf is a shortcut for Attach(err, status.Newf(...))
func Attachf(err error, code codes.Code, format string, args ...interface{}) error {
	return Attach(err, status.Newf(code, format, args...))
}

// FromError returns a Status attached to err if it was attached using this
// package. Otherwise, ok is false and a Status is returned with codes.Unknown
// and the original error message.
func FromError(err error) (st *status.Status, ok bool) {
	if err == nil {
		return nil, true
	}
	v, ok := errors.TagValueIn(appStatusTagKey, err)
	if !ok {
		return status.New(codes.Unknown, err.Error()), false
	}
	return v.(*status.Status), true
}

// Convert is a convenience function which removes the need to handle the
// boolean return value from FromError.
func Convert(err error) *status.Status {
	s, _ := FromError(err)
	return s
}
