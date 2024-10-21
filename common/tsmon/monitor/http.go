// Copyright 2016 The LUCI Authors.
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

package monitor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/protobuf/jsonpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	// ProdxmonScopes is the list of oauth2 scopes needed on the http client
	// given to NewHTTPMonitor.
	ProdxmonScopes = []string{"https://www.googleapis.com/auth/prodxmon"}
)

type httpMonitor struct {
	client   *http.Client
	endpoint *url.URL
}

// NewHTTPMonitor creates a new Monitor object that sends metric to an HTTP
// (or HTTPS) endpoint.  The http client should be authenticated as required.
//
// DEPRECATED: NewGRPCMonitor should be used instead.
func NewHTTPMonitor(ctx context.Context, client *http.Client, endpoint *url.URL) (Monitor, error) {
	return &httpMonitor{
		client:   client,
		endpoint: endpoint,
	}, nil
}

func (m *httpMonitor) ChunkSize() int {
	return 500
}

func (m *httpMonitor) Send(ctx context.Context, cells []types.Cell, now time.Time) (err error) {
	// Note `now` can actually be in the past. Here we use `startTime` exclusively
	// to measure how long it takes to call Insert. So get the freshest value.
	startTime := clock.Now(ctx)
	defer func() {
		if err == nil {
			logging.Debugf(ctx, "tsmon: sent %d cells in %s", len(cells), clock.Since(ctx, startTime))
		} else {
			logging.Warningf(ctx, "tsmon: failed to send %d cells - %s", len(cells), err)
		}
	}()

	// Don't waste time on serialization if we are already too late.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Serialize the tsmon cells into protobufs.
	req := &pb.Request{
		Payload: &pb.MetricsPayload{
			MetricsCollection: SerializeCells(cells, now),
		},
	}

	// JSON encode the request.
	encoded := bytes.Buffer{}
	marshaller := jsonpb.Marshaler{}
	if err := marshaller.Marshal(&encoded, req); err != nil {
		return err
	}

	// Make the request.
	status, err := lhttp.NewRequest(ctx, m.client, nil, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", m.endpoint.String(), bytes.NewReader(encoded.Bytes()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}, func(resp *http.Response) error {
		return resp.Body.Close()
	}, func(resp *http.Response, oErr error) error {
		if resp != nil {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				logging.WithError(err).Warningf(ctx, "Failed to read error response body")
			} else {
				logging.Warningf(
					ctx, "Monitoring push failed.\nResponse body: %s\nRequest body: %s",
					body, encoded.Bytes())
			}
			resp.Body.Close()
		}
		// On HTTP 429 response (Too many requests) oErr is marked as transient and
		// returning it causes a retry. We don't want to do that. HTTP 429 is
		// received if timestamps in the request body indicate that the sampling
		// period is smaller than the configured retention period. Resending the
		// exact same body with exact same timestamps won't help. Return a fatal
		// error instead.
		if resp != nil && resp.StatusCode == 429 {
			return fmt.Errorf("giving up on HTTP 429 status")
		}
		return oErr
	})()
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("bad response status %d from endpoint %s", status, m.endpoint)
	}

	return nil
}

func (m *httpMonitor) Close() error {
	return nil
}
