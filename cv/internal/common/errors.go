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

package common

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"
)

// MostSevereError returns the most severe error in order of
// non-transient => transient => nil.
//
// Walks over potentially recursive errors.MultiError errors only.
//
// Returns only singular errors or nil if input was nil.
func MostSevereError(err error) error {
	if err == nil {
		return nil
	}
	errs, ok := err.(errors.MultiError)
	if !ok {
		return err
	}
	var firstTrans error
	for _, err := range errs {
		switch err = MostSevereError(err); {
		case err == nil:
		case !transient.Tag.In(err):
			return err
		case firstTrans == nil:
			firstTrans = err
		}
	}
	return firstTrans
}

// TQIfy converts CV error semantics to server/TQ, and logs error if necessary.
//
// Usage:
//   func tqHandler(ctx ..., payload...) error {
//     err := doStuff(ctx, ...)
//     return TQIfy{}.Error(ctx, err)
//   }
//
// Given that:
//  * TQ lib recognizes 2 error tags:
//    * TQ.Fatal => HTTP 202, no retries
//    * transient.Tag => HTTP 500, will be retried
//    * else => HTTP 429, will be retried
// OTOH, CV uses
//  * transient.Tag to treat all _transient_ situations, where retry should
//    help
//  * else => permanent errors, where retries aren't helpful.
//
// Most _transient_ situations in CV are due to expected issues such as Gerrit
// giving stale data. Getting HTTP 500s in this case is an unfortunate noise,
// which obscures other infrequent situations which are worth looking at.
type TQIfy struct {
	// KnownRetry are expected errors which will result in HTTP 429 and retries*.
	//
	// * retries may not happen if task queue configuration prevents it, e.g.
	// because task has exhausted its retry quota.
	//
	// Retry and Fatal should not match the same error, but if this happens, Retry
	// takes effect and Fatal is ignored to avoid accidental damage.
	//
	// Stack trace isn't logged, but server/tq still logs the error string.
	//
	// Must contain only leaf errors, i.e. no annotated or MultiError objects.
	KnownRetry []error
	// KnownFatal are expected errors which will result in HTTP 202 and no
	// retries.
	//
	// Stack trace isn't logged, but server/tq still logs the error string.
	//
	// Must contain only leaf errors, i.e. no annotated or MultiError objects.
	KnownFatal []error
}

func (t TQIfy) Error(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	retry := matchesErrors(err, t.KnownRetry...)
	fail := matchesErrors(err, t.KnownFatal...)
	switch {
	case retry:
		if fail {
			logging.Errorf(ctx, "BUG: invalid TQIfy config %v: error %s matched both KnownRetry and KnownFatal", t, err)
		}
		logging.Warningf(ctx, "Will retry due to anticipated error: %s", err)
		if transient.Tag.In(err) {
			// Get rid of transient tag for TQ to treat error as 429.
			return transient.Tag.Off().Apply(err)
		}
		return err

	case fail:
		logging.Warningf(ctx, "Failing due to anticipated error: %s", err)
		return tq.Fatal.Apply(err)

	case !transient.Tag.In(err):
		// Unexpected error which isn't considered retryable.
		err = tq.Fatal.Apply(err)
		fallthrough
	default:
		// Unexpected error get logged with full stacktrace.
		LogError(ctx, err)
		return err
	}
}

// TQifyError is shortcut for TQIfy{}.Error.
func TQifyError(ctx context.Context, err error) error {
	return TQIfy{}.Error(ctx, err)
}

// LogError is errors.Log with CV-specific package filtering.
//
// Logs entire error stack with ERROR severity by default.
// Logs just error with WARNING severity iff one of error (or its inner error)
// equal at least one of the given list of `expectedErrors` errors.
// This is useful if TQ handler is known to frequently fail this way.
//
// expectedErrors must contain only unwrapped errors.
func LogError(ctx context.Context, err error, expectedErrors ...error) {
	if matchesErrors(err, expectedErrors...) {
		logging.Warningf(ctx, "%s", err)
		return
	}
	errors.Log(
		ctx,
		err,
		// These packages are not useful in CV tests:
		"github.com/smartystreets/goconvey/convey",
		"github.com/jtolds/gls",
		// These packages are not useful in production:
		// TODO(tandrii): undo this once all TQ-related CLs land and stick in prod.
		// "go.chromium.org/luci/server",
		// "go.chromium.org/luci/server/tq",
		"go.chromium.org/luci/server/router",
	)
}

func matchesErrors(err error, knownErrors ...error) bool {
	omit := false
	errors.WalkLeaves(err, func(iErr error) bool {
		for _, kErr := range knownErrors {
			if iErr == kErr {
				omit = true
				return false // stop iteration
			}
		}
		return true // continue iterating
	})
	return omit
}

// IsDatastoreContention is best-effort detection of transactions aborted due to
// pessimistic concurrency control of Datastore backed by Firestore.
//
// This is fragile, because it relies on undocumented but likely rarely changed
// English description of an error.
func IsDatastoreContention(err error) bool {
	ret := false
	errors.WalkLeaves(err, func(leaf error) bool {
		s, ok := status.FromError(err)
		if ok && s.Code() == codes.Aborted && !strings.HasPrefix(s.Message(), "Aborted due to cross-transaction contention") {
			ret = true
			return false //stop
		}
		return true //continue
	})
	return ret
}
