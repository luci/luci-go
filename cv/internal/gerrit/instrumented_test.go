// Copyright 2020 The LUCI Authors.
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

package gerrit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestInstrumentedFactory(t *testing.T) {
	t.Parallel()

	ftt.Run("InstrumentedFactory works", t, func(t *ftt.Test) {
		ctx := context.Background()
		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}
		ctx = memory.Use(ctx)
		ctx, _, _ = tsmon.WithFakes(ctx)
		tsmon.GetState(ctx).SetStore(store.NewInMemory(&target.Task{}))
		ctx = authtest.MockAuthConfig(ctx)
		epoch := datastore.RoundTime(testclock.TestRecentTimeUTC)
		ctx, tclock := testclock.UseTime(ctx, epoch)

		mockResp := "" // modified in tests later.
		mockHTTPCode := http.StatusOK
		mockDelay := time.Millisecond
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tclock.Add(mockDelay)
			w.WriteHeader(mockHTTPCode)
			if mockHTTPCode == http.StatusOK {
				w.Write([]byte(mockResp))
			}
		}))
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		assert.Loosely(t, err, should.BeNil)
		gHost := u.Host

		prod, err := newProd(ctx)
		assert.Loosely(t, err, should.BeNil)
		prod.baseTransport = srv.Client().Transport
		prod.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
			return &auth.Token{
				Token:  "token",
				Expiry: tclock.Now().Add(2 * time.Minute),
			}, nil
		}

		f := InstrumentedFactory(prod)
		c1, err := f.MakeClient(ctx, gHost, "prj1")
		assert.Loosely(t, err, should.BeNil)
		c2, err := f.MakeClient(ctx, gHost, "prj2")
		assert.Loosely(t, err, should.BeNil)

		mockDelay, mockHTTPCode, mockResp = time.Second, http.StatusOK, ")]}'\n[]" // no changes.
		r1, err := c1.ListChanges(ctx, &gerritpb.ListChangesRequest{})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, r1.GetChanges(), should.BeEmpty)
		assert.Loosely(t, tsmonSentCounter(ctx, metricCount, "prj1", gHost, "ListChanges", "OK"), should.Equal(1))
		d1 := tsmonSentDistr(ctx, metricDurationMS, "prj1", gHost, "ListChanges", "OK")
		assert.That(t, d1.Sum(), should.Equal(float64(mockDelay.Milliseconds())))

		mockDelay, mockHTTPCode, mockResp = time.Millisecond, http.StatusNotFound, ""
		_, err = c2.GetChange(ctx, &gerritpb.GetChangeRequest{Number: 1})
		assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
		assert.Loosely(t, tsmonSentCounter(ctx, metricCount, "prj2", gHost, "GetChange", "NOT_FOUND"), should.Equal(1))
		d2 := tsmonSentDistr(ctx, metricDurationMS, "prj2", gHost, "GetChange", "NOT_FOUND")
		assert.That(t, d2.Sum(), should.Equal(float64(mockDelay.Milliseconds())))
	})
}

func tsmonSentDistr(ctx context.Context, m types.Metric, fieldVals ...any) *distribution.Distribution {
	d, ok := tsmon.GetState(ctx).Store().Get(ctx, m, fieldVals).(*distribution.Distribution)
	if !ok {
		panic(fmt.Errorf("either metric isn't a Distribution or nothing sent with metric fields %s", fieldVals))
	}
	return d
}

func tsmonSentCounter(ctx context.Context, m types.Metric, fieldVals ...any) int64 {
	v, ok := tsmon.GetState(ctx).Store().Get(ctx, m, fieldVals).(int64)
	if !ok {
		panic(fmt.Errorf("either metric isn't a Counter or nothing sent with metric fields %s", fieldVals))
	}
	return v
}
