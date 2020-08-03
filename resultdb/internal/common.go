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

package internal

import (
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/grpc/appstatus"
)

// CommonPostlude must be used as a postlude in all ResultDB services.
//
// Extracts a status using appstatus and returns to the requester.
// If the error is internal or unknown, logs the stack trace.
func CommonPostlude(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	return appstatus.GRPCifyAndLog(ctx, err)
}

// AssertUTC panics if t is not UTC.
func AssertUTC(t time.Time) {
	if t.Location() != time.UTC {
		panic("not UTC")
	}
}
