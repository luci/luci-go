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

package recorder

import (
	"testing"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"

	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const expectedResultExpiration = 60 * day

func TestMain(m *testing.M) {
	testutil.SpannerTestMain(m)
}

func newTestRecorderServer() *pb.DecoratedRecorder {
	return newTestRecorderServerWithClients(nil, nil)
}

func newTestRecorderServerWithClients(casClient repb.ContentAddressableStorageClient, bqClient BQExportClient) *pb.DecoratedRecorder {
	opts := Options{ExpectedResultsExpiration: expectedResultExpiration}
	return NewRecorderServer(opts, casClient, bqClient)
}
