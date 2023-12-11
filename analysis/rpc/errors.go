// Copyright 2022 The LUCI Authors.
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

package rpc

import (
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"
)

// invalidArgumentError annotates err as having an invalid argument.
// The error message is shared with the requester as is.
//
// Note that this differs from FailedPrecondition. It indicates arguments
// that are problematic regardless of the state of the system
// (e.g., a malformed file name).
func invalidArgumentError(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "%s", err)
}

// failedPreconditionError annotates err as failing a precondition for the
// operation. The error message is shared with the requester as is.
//
// See codes.FailedPrecondition for more context about when this
// should be used compared to invalid argument.
func failedPreconditionError(err error) error {
	return appstatus.Attachf(err, codes.FailedPrecondition, "%s", err)
}
