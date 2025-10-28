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
	"strings"
	"time"

	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
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

// Snippets of HTTP 403 or HTTP 400 error messages indicating there's something
// wrong with billing to a user project.
var billingErrs = []string{
	`does not have serviceusage.services.use access`,
	`billing account for the owning project is disabled`,
	`project specified in the request is invalid`,
}

// withRetry executes a Google Storage API call, retrying on transient errors.
//
// If a request reached GS, but the service replied with an error, the
// corresponding HTTP status code can be extracted from the error via
// StatusCode(err). The error is also tagged as transient based on the code:
// response with HTTP statuses >=500 and 429 are considered transient errors.
//
// If the request never reached GS, StatusCode(err) would return 0 and the error
// will be tagged as transient.
//
// Additionally attaches gRPC status tag matching the semantic meaning of the
// error.
func withRetry(ctx context.Context, call func() error) error {
	err := retry.Retry(ctx, transient.Only(func() retry.Iterator {
		it := retryPolicy
		return &it
	}), func() error {
		err := call()
		if err == nil {
			return nil
		}

		// RestartUploadError errors are fatal and should be passed unannotated.
		var restartErr *RestartUploadError
		if errors.As(err, &restartErr) {
			return err
		}

		// Any other error that is not googleapi.Error means we failed to call GCS.
		var apiErr *googleapi.Error
		if !errors.As(err, &apiErr) {
			return transient.Tag.Apply(errors.Fmt("failed to call GS: %w", err))
		}

		logging.Infof(ctx, "GS replied with HTTP code %d", apiErr.Code)
		logging.Debugf(ctx, "full response body:\n%s", apiErr.Body)

		// Note we purposefully don't wrap apiErr (i.e. we use %s instead of %w),
		// because, despite using HTTP API, apiErr secretly has GRPCStatus() inside
		// and this particular status error is getting erroneously picked up by
		// status.FromError(...).
		err = errors.Fmt("GS replied with HTTP code %d: %s", apiErr.Code, apiErr.Message)
		err = StatusCodeTag.ApplyValue(err, apiErr.Code)

		// Retry only on 429 and 5xx responses, according to
		// https://cloud.google.com/storage/docs/exponential-backoff.
		if apiErr.Code == 429 || apiErr.Code >= 500 {
			return transient.Tag.Apply(err)
		}

		return err
	}, func(err error, d time.Duration) {
		logging.WithError(err).Errorf(ctx, "Transient error when accessing GS. Retrying in %s...", d)
	})

	if err == nil {
		return nil
	}
	if transient.Tag.In(err) {
		return grpcutil.InternalTag.Apply(err)
	}

	// Pick a gRPC status tag. This eventually bubbles up to RPC clients.
	switch StatusCode(err) {
	case 0:
		return err // not an googleapi.Error, pass it through
	case http.StatusNotFound:
		return grpcutil.NotFoundTag.Apply(err)
	case http.StatusForbidden, http.StatusBadRequest:
		msg := err.Error()
		for _, snippet := range billingErrs {
			if strings.Contains(msg, snippet) {
				return grpcutil.PermissionDeniedTag.Apply(
					errors.Fmt("check billing project configuration: %w", err),
				)
			}
		}
		// All other 403 and 400 errors are unexpected.
		return grpcutil.InternalTag.Apply(err)
	default:
		// Any other error means something unexpected happened.
		return grpcutil.InternalTag.Apply(err)
	}
}
