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
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
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
//
//	func tqHandler(ctx ..., payload...) error {
//	  err := doStuff(ctx, ...)
//	  return TQIfy{}.Error(ctx, err)
//	}
//
// Given that:
//   - TQ lib recognizes these error kinds:
//   - tq.Ignore => HTTP 204, no retries
//   - tq.Fatal => HTTP 202, no retries, but treated with alertable in our
//     monitoring configuration;
//   - transient.Tag => HTTP 500, will be retried;
//   - else => HTTP 429, will be retried.
//
// OTOH, CV uses
//   - transient.Tag to treat all _transient_ situations, where retry should
//     help
//   - else => permanent errors, where retries aren't helpful.
//
// Most _transient_ situations in CV are due to expected issues such as Gerrit
// giving stale data. Getting HTTP 500s in this case is an unfortunate noise,
// which obscures other infrequent situations which are worth looking at.
type TQIfy struct {
	// KnownRetry are expected errors which will result in HTTP 429 and retries.
	//
	// Retries may not happen if task queue configuration prevents it, e.g.
	// because task has exhausted its retry quota.
	//
	// KnownRetry and KnownIgnore should not match the same error, but if this
	// happens, Retry takes effect and KnownIgnore is ignored to avoid accidental
	// loss of tasks.
	//
	// Must contain only leaf errors, i.e. no annotated or MultiError objects.
	KnownRetry []error
	// KnownRetryTags are similar to `KnowRetry`, but are the expected tags that
	// the CV error should be tagged with.
	//
	// Must not contain `transient.Tag`.
	KnownRetryTags []errtag.Tag[bool]
	// NeverRetry instructs TQ not to retry on any unexpected error.
	//
	// Transient error will be tagged with `tq.Ignore` while non-transient error
	// will be tagged with `tq.Fatal`. See the struct doc for what each tag means.
	//
	// Recommend to use this flag when tasks are executed periodically in short
	// interval (e.g. refresh config task) where as retrying failed task is not
	// necessary.
	//
	// Mutually exclusive with `KnownRetry` and `KnownRetryTags`.
	NeverRetry bool
	// KnownIgnore are expected errors which will result in HTTP 204 and no
	// retries.
	//
	// Must contain only leaf errors, i.e. no annotated or MultiError objects.
	KnownIgnore []error
	// KnownIgnoreTags are similar to `KnownIgnore`, but are the expected tags
	// that the CV error should be tagged with.
	//
	// Must not contain `transient.Tag`.
	KnownIgnoreTags []errtag.Tag[bool]
}

func (t TQIfy) Error(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	retry := false
	switch {
	case !t.NeverRetry:
		retry = matchesErrors(err, t.KnownRetry...) || matchesErrorTags(err, t.KnownRetryTags...)
	case len(t.KnownRetry) > 0 || len(t.KnownRetryTags) > 0:
		panic("NeverRetry and KnownRetry/KnownRetryTags are mutually exclusive")
	}
	ignore := matchesErrors(err, t.KnownIgnore...) || matchesErrorTags(err, t.KnownIgnoreTags...)
	switch {
	case retry:
		if ignore {
			logging.Errorf(ctx, "BUG: invalid TQIfy config %v: error %s matched both KnownRetry and KnownIgnore", t, err)
		}
		logging.Warningf(ctx, "Will retry due to anticipated error: %s", err)
		if transient.Tag.In(err) {
			// Get rid of transient tag for TQ to treat error as 429.
			return transient.Tag.ApplyValue(err, false)
		}
		return err

	case ignore:
		logging.Warningf(ctx, "Failing due to anticipated error: %s", err)
		return tq.Ignore.Apply(err)

	default:
		// Unexpected error is logged with full stacktrace.
		LogError(ctx, err)
		switch {
		case !transient.Tag.In(err):
			return tq.Fatal.Apply(err)
		case t.NeverRetry:
			return tq.Ignore.Apply(err)
		default:
			return err
		}
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

	// Annotate error to get full stack trace of the caller of the LogError.
	err = errors.Annotate(err, "common.LogError").Err()

	errors.Log(
		ctx,
		err,
		// These packages are not useful in production:
		"go.chromium.org/luci/server",
		"go.chromium.org/luci/server/tq",
		"go.chromium.org/luci/server/router",
	)
}

func matchesErrors(err error, knownErrors ...error) bool {
	for _, kErr := range knownErrors {
		if errors.Is(err, kErr) {
			return true
		}
	}
	return false
}

func matchesErrorTags(err error, knownTags ...errtag.Tag[bool]) bool {
	for _, kTag := range knownTags {
		if kTag.Is(transient.Tag) {
			panic("knownTags MUST not contain transient.Tag")
		}
		if kTag.In(err) {
			return true
		}
	}
	return false
}

// DSContentionTag when set indicates Datastore contention.
//
// It's set on errors by parts of CV which are especially prone to DS contention
// to reduce noise in logs and for more effective retries.
var DSContentionTag = errtag.Make("Datastore Contention", true)

// IsDatastoreContention is best-effort detection of transactions aborted due to
// pessimistic concurrency control of Datastore backed by Firestore.
//
// This is fragile, because it relies on undocumented but likely rarely changed
// English description of an error.
func IsDatastoreContention(err error) bool {
	if DSContentionTag.In(err) {
		return true
	}
	ret := false
	errors.WalkLeaves(err, func(leaf error) bool {
		if leaf == datastore.ErrConcurrentTransaction {
			ret = true
			return false //stop
		}
		s, ok := status.FromError(leaf)
		if ok && s.Code() == codes.Aborted && strings.Contains(s.Message(), "Aborted due to cross-transaction contention") {
			ret = true
			return false //stop
		}
		return true //continue
	})
	return ret
}
