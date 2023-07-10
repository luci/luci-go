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

package assertions

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/smarty/assertions"
	"github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
)

// ShouldHaveAppStatus asserts that error `actual` has an
// application-specific status and it matches the expectations.
// See ShouldBeLikeStatus for the format of `expected`.
// See appstatus package for application-specific statuses.
func ShouldHaveAppStatus(actual any, expected ...any) string {
	if ret := assertions.ShouldImplement(actual, (*error)(nil)); ret != "" {
		return ret
	}
	actualStatus, ok := appstatus.Get(actual.(error))
	if !ok {
		return fmt.Sprintf("expected error %q to have an explicit application status", actual)
	}

	return ShouldBeLikeStatus(actualStatus, expected...)
}

// ShouldHaveRPCCode is a goconvey assertion, asserting that the supplied
// "actual" value has a gRPC code value and, optionally, errors like a supplied
// message string.
//
// If no "expected" arguments are supplied, ShouldHaveRPCCode will assert that
// the result is codes.OK.
//
// The first "expected" argument, if supplied, is the gRPC codes.Code to assert.
//
// A second "expected" string may be optionally included. If included, the
// gRPC error message is asserted to contain the expected string using
// convey.ShouldContainSubstring.
func ShouldHaveRPCCode(actual any, expected ...any) string {
	aerr, ok := actual.(error)
	if !(ok || actual == nil) {
		return "actual argument must be an error."
	}

	var (
		ecode   codes.Code
		errLike string
	)
	switch len(expected) {
	case 2:
		var ok bool
		if errLike, ok = expected[1].(string); !ok {
			return fmt.Sprintf("The expected error substring must be a string, not a %T", expected[1])
		}
		fallthrough

	case 1:
		var ok bool
		if ecode, ok = expected[0].(codes.Code); !ok {
			return fmt.Sprintf("The code must be a codes.Code, not a %T", expected[0])
		}

	case 0:
		ecode = codes.OK

	default:
		return "Expected argument must have the form: [codes.Code[string]]"
	}

	if acode := grpcutil.Code(aerr); acode != ecode {
		return fmt.Sprintf("expected gRPC code %q (%d), not %q (%d), type %T: %v",
			ecode, ecode, acode, acode, actual, actual)
	}

	if errLike != "" {
		return convey.ShouldContainSubstring(status.Convert(aerr).Message(), errLike)
	}
	return ""
}

// ShouldBeRPCOK asserts that "actual" is an error that has a gRPC code value
// of codes.OK.
//
// Note that "nil" has an codes.OK value.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCOK(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.OK, expected)...)
}

// ShouldBeRPCInvalidArgument asserts that "actual" is an error that has a gRPC
// code value of codes.InvalidArgument.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCInvalidArgument(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.InvalidArgument, expected)...)
}

// ShouldBeRPCInternal asserts that "actual" is an error that has a gRPC code
// value of codes.Internal.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCInternal(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.Internal, expected)...)
}

// ShouldBeRPCUnknown asserts that "actual" is an error that has a gRPC code
// value of codes.Unknown.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCUnknown(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.Unknown, expected)...)
}

// ShouldBeRPCNotFound asserts that "actual" is an error that has a gRPC code
// value of codes.NotFound.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCNotFound(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.NotFound, expected)...)
}

// ShouldBeRPCPermissionDenied asserts that "actual" is an error that has a gRPC
// code value of codes.PermissionDenied.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCPermissionDenied(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.PermissionDenied, expected)...)
}

// ShouldBeRPCAlreadyExists asserts that "actual" is an error that has a gRPC
// code value of codes.AlreadyExists.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCAlreadyExists(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.AlreadyExists, expected)...)
}

// ShouldBeRPCUnauthenticated asserts that "actual" is an error that has a gRPC
// code value of codes.Unauthenticated.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCUnauthenticated(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.Unauthenticated, expected)...)
}

// ShouldBeRPCFailedPrecondition asserts that "actual" is an error that has a gRPC
// code value of codes.FailedPrecondition.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCFailedPrecondition(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.FailedPrecondition, expected)...)
}

// ShouldBeRPCAborted asserts that "actual" is an error that has a gRPC
// code value of codes.Aborted.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCAborted(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.Aborted, expected)...)
}

// ShouldBeRPCDeadlineExceeded asserts that "actual" is an error that has a gRPC
// code value of codes.DeadlineExceeded.
//
// One additional "expected" string may be optionally included. If included, the
// gRPC error's message is asserted to contain the expected string.
func ShouldBeRPCDeadlineExceeded(actual any, expected ...any) string {
	return ShouldHaveRPCCode(actual, prepend(codes.DeadlineExceeded, expected)...)
}

func prepend(c codes.Code, exp []any) []any {
	args := make([]any, len(exp)+1)
	args[0] = c
	copy(args[1:], exp)
	return args
}

// ShouldBeLikeStatus asserts that *status.Status `actual` has code
// `expected[0]`, that the actual message has a substring `expected[1]` and
// that the status details in expected[2:] as present in the actual status.
//
// len(expected) must be at least 1.
//
// Example:
//
//	// err must have a NotFound status
//	So(s, ShouldBeLikeStatus, codes.NotFound)
//
//	// and its message must contain "item not found"
//	So(s, ShouldBeLikeStatus, codes.NotFound, "item not found")
//
//	// and it must have a DebugInfo detail.
//	So(s, ShouldBeLikeStatus, codes.NotFound, "item not found", &errdetails.DebugInfo{Details: "x"})
func ShouldBeLikeStatus(actual any, expected ...any) string {
	if ret := assertions.ShouldHaveSameTypeAs(actual, (*status.Status)(nil)); ret != "" {
		return ret
	}

	if ret := assertions.ShouldNotBeEmpty(expected); ret != "" {
		return ret
	}

	actualStatus := actual.(*status.Status)

	if ret := assertions.ShouldEqual(actualStatus.Code(), expected[0]); ret != "" {
		return ret
	}

	if len(expected) == 1 {
		return ""
	}

	if ret := assertions.ShouldContainSubstring(actualStatus.Message(), expected[1]); ret != "" {
		return ret
	}

	if len(expected) == 2 {
		return ""
	}

	// Serialize actual details to strings as compact text proto.
	actualDetails := actualStatus.Details()
	presentDetails := stringset.New(len(actualDetails))
	for _, d := range actualDetails {
		presentDetails.Add(proto.CompactTextString(d.(proto.Message)))
	}

	// Then assert presence of each expected detail.
	for _, d := range expected[2:] {
		if ret := assertions.ShouldImplement(d, (*proto.Message)(nil)); ret != "" {
			return ret
		}
		eTxt := proto.CompactTextString(d.(proto.Message))
		if !presentDetails.Has(eTxt) {
			return fmt.Sprintf("expected presence of status detail %q, got %q", eTxt, presentDetails.ToSlice())
		}
	}

	return ""
}

// ShouldHaveGRPCStatus asserts that error `actual` has a GRPC status and it
// matches the expectations.
// See ShouldBeStatusLike for the format of `expected`.
// The status is extracted using status.FromError.
func ShouldHaveGRPCStatus(actual any, expected ...any) string {
	if ret := assertions.ShouldImplement(actual, (*error)(nil)); ret != "" {
		return ret
	}
	actualStatus, ok := status.FromError(actual.(error))
	if !ok {
		return fmt.Sprintf("expected error %q to have a GRPC status", actual)
	}

	return ShouldBeLikeStatus(actualStatus, expected...)
}
