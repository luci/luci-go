// Copyright 2015 The LUCI Authors.
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
	"net/http"

	"go.chromium.org/luci/common/errors"
)

// ErrorCode represents milo's internal error code system. The subsystems in
// milo can attach an ErrorCode to an error (using ErrorTag), and then the html
// and grpc frontends know how to render the error in a way that will help the
// user.
type ErrorCode int

const (
	// CodeUnknown means an error happened, but we weren't able to classify it.
	CodeUnknown ErrorCode = iota

	// CodeUnauthorized occurs when a user isn't logged in, but needs to be to
	// proceed further.
	CodeUnauthorized

	// CodeNoAccess occurs when the currently logged-in user is not permitted to
	// see the resource.
	CodeNoAccess

	// CodeNotFound occurs when we weren't able to find the build that the
	// user asked for.
	CodeNotFound

	// CodeParameterError occurs when one or more of the user-provided parameters
	// was malformed.
	CodeParameterError
)

var httpCode = map[ErrorCode]int{
	CodeUnauthorized:   http.StatusUnauthorized,
	CodeNoAccess:       http.StatusForbidden,
	CodeNotFound:       http.StatusNotFound,
	CodeParameterError: http.StatusBadRequest,
}

// HTTPStatus returns an HTTP Status code corresponding to this ErrorCode.
func (c ErrorCode) HTTPStatus() int {
	if ret, ok := httpCode[c]; ok {
		return ret
	}
	return http.StatusInternalServerError
}

// Tag returns an errors.TagValue for this code.
func (c ErrorCode) Tag() errors.TagValue {
	return ErrorTag.With(c)
}

// GenerateErrorTagValue implements errors.TagValueGenerator so that ErrorCodes
// can be used like:
//   errors.Annotate(err).Tag(CodeNotFound)
//   errors.New("terrible thing", CodeNotFound)
func (c ErrorCode) GenerateErrorTagValue() errors.TagValue {
	return ErrorTag.With(c)
}

type errorTag struct{ Key errors.TagKey }

func (t *errorTag) In(err error) ErrorCode {
	v, ok := errors.TagValueIn(t.Key, err)
	if ok {
		return v.(ErrorCode)
	}
	return CodeUnknown
}

func (t *errorTag) With(value ErrorCode) errors.TagValue {
	return errors.TagValue{Key: t.Key, Value: value}
}

// ErrorTag should be used to tag errors in the milo ecosystem to categorize
// them so they can be rendered properly in the frontend.
var ErrorTag = errorTag{errors.NewTagKey("holds a milo ErrorCode")}
