// Copyright 2026 The LUCI Authors.
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

package backend

import (
	"net/http"
	"testing"

	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestErrors(t *testing.T) {
	t.Parallel()

	ftt.Run("isStockoutGoogleAPIError", t, func(t *ftt.Test) {
		t.Run("nil error", func(t *ftt.Test) {
			stockout, reason := isStockoutGoogleAPIError(nil)
			assert.Loosely(t, stockout, should.BeFalse)
			assert.Loosely(t, reason, should.Equal(errReasonHTTPError))
		})

		t.Run("HTTP 503 Service Unavailable", func(t *ftt.Test) {
			stockout, reason := isStockoutGoogleAPIError(&googleapi.Error{
				Code: http.StatusServiceUnavailable,
			})
			assert.Loosely(t, stockout, should.BeTrue)
			assert.Loosely(t, reason, should.Equal(errReasonHTTPError))
		})

		t.Run("explicit stockout reason", func(t *ftt.Test) {
			stockout, reason := isStockoutGoogleAPIError(&googleapi.Error{
				Code: http.StatusConflict,
				Errors: []googleapi.ErrorItem{
					{Reason: errCodeZoneResourcePoolExhausted},
				},
			})
			assert.Loosely(t, stockout, should.BeTrue)
			assert.Loosely(t, reason, should.Equal(errCodeZoneResourcePoolExhausted))
		})

		t.Run("stockout message without explicit reason", func(t *ftt.Test) {
			stockout, reason := isStockoutGoogleAPIError(&googleapi.Error{
				Code: http.StatusBadRequest,
				Errors: []googleapi.ErrorItem{
					{Message: "The zone 'projects/p/zones/z' does not have enough resources available to fulfill the request."},
				},
			})
			assert.Loosely(t, stockout, should.BeTrue)
			assert.Loosely(t, reason, should.Equal(errCodeZoneResourcePoolExhausted))
		})

		t.Run("rate limit error is not stockout", func(t *ftt.Test) {
			stockout, reason := isStockoutGoogleAPIError(&googleapi.Error{
				Code: http.StatusServiceUnavailable,
				Errors: []googleapi.ErrorItem{
					{Reason: errReasonRateLimitExceeded, Message: errMsgRateLimitExceeded},
				},
			})
			assert.Loosely(t, stockout, should.BeFalse)
			assert.Loosely(t, reason, should.Equal(errReasonRateLimitExceeded))
		})

		t.Run("non-stockout HTTP error", func(t *ftt.Test) {
			stockout, reason := isStockoutGoogleAPIError(&googleapi.Error{
				Code: http.StatusForbidden,
				Errors: []googleapi.ErrorItem{
					{Reason: errCodeQuotaExceeded},
				},
			})
			assert.Loosely(t, stockout, should.BeFalse)
			assert.Loosely(t, reason, should.Equal(errCodeQuotaExceeded))
		})
	})

	ftt.Run("isStockoutOperationErrors", t, func(t *ftt.Test) {
		t.Run("ZONE_RESOURCE_POOL_EXHAUSTED code", func(t *ftt.Test) {
			stockout, reason := isStockoutOperationErrors([]CommonOpError{
				{Code: errCodeZoneResourcePoolExhausted, Message: errMsgZoneResourcePoolExhausted},
			})
			assert.Loosely(t, stockout, should.BeTrue)
			assert.Loosely(t, reason, should.Equal(errCodeZoneResourcePoolExhausted))
		})

		t.Run("ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS code", func(t *ftt.Test) {
			stockout, reason := isStockoutOperationErrors([]CommonOpError{
				{Code: errCodeZoneResourcePoolExhaustedWithDetails, Message: errMsgZoneResourcePoolExhausted},
			})
			assert.Loosely(t, stockout, should.BeTrue)
			assert.Loosely(t, reason, should.Equal(errCodeZoneResourcePoolExhaustedWithDetails))
		})

		t.Run("stockout message only", func(t *ftt.Test) {
			stockout, reason := isStockoutOperationErrors([]CommonOpError{
				{Message: "The zone does not have enough resources available."},
			})
			assert.Loosely(t, stockout, should.BeTrue)
			assert.Loosely(t, reason, should.Equal(errCodeZoneResourcePoolExhausted))
		})

		t.Run("non-stockout operation error", func(t *ftt.Test) {
			stockout, reason := isStockoutOperationErrors([]CommonOpError{
				{Code: errCodeQuotaExceeded, Message: "Quota 'CPUS' exceeded."},
			})
			assert.Loosely(t, stockout, should.BeFalse)
			assert.Loosely(t, reason, should.Equal(errCodeQuotaExceeded))
		})
	})
}
