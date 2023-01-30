// Copyright 2019 The LUCI Authors.
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

// Package appstatus can attach/reterieve an application-specific response
// status to/from an error. It designed to prevent accidental exposure of
// internal statuses to RPC clients, for example Spanner's statuses.
//
// # Attaching a status
//
// Use ToError, Error and Errorf to create new status-annotated errors.
// Use Attach and Attachf to annotate existing errors with a status.
//
//	if req.PageSize < 0  {
//	   return appstatus.Errorf(codes.InvalidArgument, "page size cannot be negative")
//	}
//	if err := checkState(); err != nil {
//	  return appstatus.Attachf(err, codes.PreconditionFailed, "invalid state")
//	}
//
// This may be done deep in the function call hierarchy.
//
// Do not use appstatus in code where you don't explicitly intend to return a
// specific status. This is unnecessary because any unrecognized error is
// treated as internal and because it is explicitly prohibited to attach a
// status to an error chain multiple times. When a status is attached, the
// package supports its propagation all the way to the requester
// unless there is code that explicitly throws it away.
//
// # Returning a status
//
// Use GRPCifyAndLog right before returning the error from a gRPC method
// handler. Usually it is done in a Postlude of a service decorator, see
// ../cmd/svcdec.
//
//	func NewMyServer() pb.MyServer {
//	  return &pb.DecoratedMyServer{
//	    Service:  &actualImpl{},
//	    Postlude: func(ctx context.Context, methodName string, rsp proto.Message, err error) error {
//	      return appstatus.GRPCifyAndLog(ctx, err)
//	    },
//	  }
//	}
//
// It recognizes only appstatus-annotated errors and treats any other error as
// internal. This behavior is important to avoid accidentally returning errors
// from Spanner or other client library that also uses grpc/status package to
// communicate status code from *other* services. For example, if a there is a
// typo in Spanner SQL statement, spanner package may return a status-annotated
// errors with code NotFound. In this case, our RPC must respond with internal
// error and not NotFound.
package appstatus
