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

package resultdb

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.SpannerTestMain(m)
}

func newTestResultDBService() *resultDBServer {
	return &resultDBServer{
		generateIsolateURL: func(ctx context.Context, host, ns, digest string) (u *url.URL, expiration time.Time, err error) {
			u = &url.URL{
				Scheme: "http",
				Host:   "results.usercontent.example.com",
				Path:   fmt.Sprintf("/isolate/%s/%s/%s", host, ns, digest),
			}
			expiration = clock.Now(ctx).UTC().Add(time.Hour)
			return
		},
	}
}
