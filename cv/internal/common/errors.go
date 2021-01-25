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

// TQifyError does final error processing before returning from a TQ handler.
//
// * logs entire error stack with ERROR severity by default;
//   logs just error with WARNING severity iff one of error's leaf nodes matches
//   at least one of the given list of `omitStackTraceFor` errors. This is
//   useful if TQ handler is known to frequently fail this way.
// * non-transient errors are tagged with tq.Fatal to avoid retries.
//
// omitStackTraceFor must contain only unwrapped errors.
func TQifyError(ctx context.Context, err error, omitStackTraceFor ...error) error {
	if err == nil {
		return nil
	}

	stackTrace := true
	errors.WalkLeaves(err, func(leafError error) bool {
		for _, e := range omitStackTraceFor {
			if leafError == e {
				stackTrace = false
				return false // stop
			}
		}
		return true // continue iterating leaf nodes
	})

	if !stackTrace {
		logging.Warningf(ctx, "%s", err)
	} else {
		LogError(ctx, err)
	}
	if !transient.Tag.In(err) {
		err = tq.Fatal.Apply(err)
	}
	return err
}

// LogError is errors.Log with CV-specific package filtering.
func LogError(ctx context.Context, err error) {
	errors.Log(
		ctx,
		err,
		// These packages are not useful in CV tests:
		"github.com/smartystreets/goconvey/convey",
		"github.com/jtolds/gls",
		// These packages are not useful in production:
		"go.chromium.org/luci/server",
		"go.chromium.org/luci/server/tq",
		"go.chromium.org/luci/server/router",
	)
}
