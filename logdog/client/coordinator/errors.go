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

package coordinator

import (
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	// ErrNoAccess is returned from methods to indicate that the requested
	// operation could not be performed because the user does not have access
	// to that data or function.
	ErrNoAccess = errors.New("coordinator: no access")

	// ErrNoSuchStream is returned when the requested strem path does not exist.
	ErrNoSuchStream = errors.New("coordinator: no such stream")
)

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	switch grpc.Code(err) {
	case codes.NotFound:
		return ErrNoSuchStream

	case codes.Unauthenticated, codes.PermissionDenied:
		return ErrNoAccess

	default:
		return err
	}
}
