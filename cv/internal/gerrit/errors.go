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

package gerrit

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
)

var ErrStaleData = transient.Tag.Apply(errors.New("fetched stale Gerrit data"))
var ErrGerritDeadlineExceeded = transient.Tag.Apply(errors.New("Gerrit took too long to respond"))
var ErrOutOfQuota = transient.Tag.Apply(errors.New("out of Gerrit Quota"))

// UnhandledError is used to process and annotate Gerrit errors.
func UnhandledError(ctx context.Context, err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	err = errors.Fmt("%s: %w", msg, err)
	switch code := grpcutil.Code(err); code {
	case
		codes.OK,
		codes.PermissionDenied,
		codes.NotFound,
		codes.FailedPrecondition:
		// These must be handled before.
		logging.Errorf(ctx, "FIXME unhandled Gerrit error: %s while %s", err, msg)
		return err

	case
		codes.InvalidArgument,
		codes.Unauthenticated:
		// This must not happen in practice unless there is a bug in CV or Gerrit.
		logging.Errorf(ctx, "FIXME bug in CV: %s while %s", err, msg)
		return err

	case codes.Unimplemented:
		// This shouldn't happen in production, but may happen in development
		// if gerrit.NewRESTClient doesn't actually implement fully the option
		// or entire method that CV is coded to work with.
		logging.Errorf(ctx, "FIXME likely bug in CV: %s while %s", err, msg)
		return err

	case codes.ResourceExhausted:
		return errors.Fmt(msg+": %w", ErrOutOfQuota)
	case codes.DeadlineExceeded:
		return errors.Fmt(msg+": %w", ErrGerritDeadlineExceeded)

	default:
		// Assume transient. If this turns out non-transient, then its code must be
		// handled explicitly above.
		return transient.Tag.Apply(err)
	}
}
