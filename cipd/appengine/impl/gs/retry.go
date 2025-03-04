// Copyright 2017 The LUCI Authors.
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

package gs

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

// StatusCodeTag holds an http status code.
var StatusCodeTag = errtag.Make("Google Storage API Status Code", 0)

// StatusCode returns 200 for a nil error, and otherwise returns
// StatusCodeTag.Value(err).
func StatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}
	return StatusCodeTag.ValueOrDefault(err)
}

// retryPolicy is a copy of the iterator returned by retry.Default as of 25Q1
// - but we want to make the downloadAttempts metric harmonize with
// retryPolicy.Limited.Retries so we had to duplicate it here.
var retryPolicy = retry.ExponentialBackoff{
	Limited: retry.Limited{
		Delay: 200 * time.Millisecond,
		// NOTE: This retry counter shows up as a metric field in downloadCallCount;
		// If you adjust this upwards, keep this in mind, or put a cap on the field
		// at lower value.
		Retries: 10,
	},
	MaxDelay:   10 * time.Second,
	Multiplier: 2,
}

// withRetry executes a Google Storage API call, retrying on transient errors.
//
// If request reached GS, but the service replied with an error, the
// corresponding HTTP status code can be extracted from the error via
// StatusCode(err). The error is also tagged as transient based on the code:
// response with HTTP statuses >=500 and 429 are considered transient errors.
//
// If the request never reached GS, StatusCode(err) would return 0 and the error
// will be tagged as transient.
func withRetry(ctx context.Context, call func() error) error {
	return retry.Retry(ctx, transient.Only(func() retry.Iterator {
		it := retryPolicy
		return &it
	}), func() error {
		err := call()
		if err == nil {
			return nil
		}
		apiErr, _ := err.(*googleapi.Error)
		if apiErr == nil {
			// RestartUploadError errors are fatal and should be passed unannotated.
			if _, ok := err.(*RestartUploadError); ok {
				return err
			}
			return errors.Annotate(err, "failed to call GS").Tag(transient.Tag).Err()
		}
		logging.Infof(ctx, "GS replied with HTTP code %d", apiErr.Code)
		logging.Debugf(ctx, "full response body:\n%s", apiErr.Body)
		ann := errors.Annotate(err, "GS replied with HTTP code %d", apiErr.Code).
			Tag(StatusCodeTag.WithDefault(apiErr.Code))
		// Retry only on 429 and 5xx responses, according to
		// https://cloud.google.com/storage/docs/exponential-backoff.
		if apiErr.Code == 429 || apiErr.Code >= 500 {
			ann.Tag(transient.Tag)
		}
		return ann.Err()
	}, func(err error, d time.Duration) {
		logging.WithError(err).Errorf(ctx, "Transient error when accessing GS. Retrying in %s...", d)
	})
}
