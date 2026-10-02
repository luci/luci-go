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
	"strings"

	"google.golang.org/api/googleapi"
)

// GCP Compute API error codes, reasons, and normalized metric failure reasons.
const (
	errCodeZoneResourcePoolExhausted            = "ZONE_RESOURCE_POOL_EXHAUSTED"
	errCodeZoneResourcePoolExhaustedWithDetails = "ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS"
	errCodeResourceExhausted                    = "RESOURCE_EXHAUSTED"
	errCodeQuotaExceeded                        = "QUOTA_EXCEEDED"
	errReasonRateLimitExceeded                  = "rateLimitExceeded"
	errReasonHTTPError                          = "HTTP_ERROR"
	errReasonOperationError                     = "OPERATION_ERROR"
	errReasonNotFound                           = "NOT_FOUND"
)

// Substrings matched against GCP Compute API error messages when classifying
// stockout and rate-limit failures.
const (
	errMsgZoneResourcePoolExhausted   = "does not have enough resources available"
	errMsgRateLimitExceeded           = "Rate Limit Exceeded"
	errMsgQueriesPerUserPer100Seconds = "Queries per user per 100 seconds"
)

// rateLimitExceeded returns whether the given *googleapi.Error contains a rate
// limit error.
func rateLimitExceeded(err *googleapi.Error) bool {
	for _, e := range err.Errors {
		switch {
		case strings.Contains(e.Message, errMsgQueriesPerUserPer100Seconds):
			return true
		case strings.Contains(e.Message, errMsgRateLimitExceeded):
			return true
		case strings.Contains(e.Message, errReasonRateLimitExceeded):
			return true
		case strings.Contains(e.Reason, errReasonRateLimitExceeded):
			return true
		}
	}
	return false
}

// isStockoutCode returns whether the given error code or reason string
// indicates a hard GCP capacity stockout.
func isStockoutCode(s string) bool {
	return strings.Contains(s, errCodeZoneResourcePoolExhausted) ||
		strings.Contains(s, errCodeResourceExhausted)
}

// isStockoutMessage returns whether the given error message string indicates a
// hard GCP capacity stockout.
func isStockoutMessage(s string) bool {
	return isStockoutCode(s) ||
		strings.Contains(s, errMsgZoneResourcePoolExhausted)
}

// isStockoutGoogleAPIError returns whether the given *googleapi.Error indicates
// a hard GCP capacity stockout (HTTP 503 Service Unavailable or an explicit
// ZONE_RESOURCE_POOL_EXHAUSTED / RESOURCE_EXHAUSTED error) along with the
// normalized failure reason.
func isStockoutGoogleAPIError(gerr *googleapi.Error) (bool, string) {
	if gerr == nil {
		return false, errReasonHTTPError
	}
	reason := errReasonHTTPError
	if len(gerr.Errors) > 0 && gerr.Errors[0].Reason != "" {
		reason = gerr.Errors[0].Reason
	}
	if rateLimitExceeded(gerr) {
		return false, reason
	}
	for _, err := range gerr.Errors {
		if isStockoutCode(err.Reason) {
			return true, err.Reason
		}
		if isStockoutMessage(err.Message) {
			if reason == errReasonHTTPError {
				reason = errCodeZoneResourcePoolExhausted
			}
			return true, reason
		}
	}
	if gerr.Code == http.StatusServiceUnavailable {
		return true, reason
	}
	return false, reason
}

// isStockoutOperationErrors returns whether the operation errors include a hard
// GCP capacity stockout and the primary failure reason.
func isStockoutOperationErrors(opErrors []CommonOpError) (bool, string) {
	reason := errReasonOperationError
	for _, err := range opErrors {
		if err.Code != "" && reason == errReasonOperationError {
			reason = err.Code
		}
		if isStockoutCode(err.Code) {
			return true, err.Code
		}
		if isStockoutMessage(err.Message) {
			if reason == errReasonOperationError {
				reason = errCodeZoneResourcePoolExhausted
			}
			return true, reason
		}
	}
	return false, reason
}
