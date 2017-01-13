// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package monitor

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/golang/protobuf/jsonpb"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/lhttp"
	"github.com/luci/luci-go/common/logging"
	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto_v2"
	"github.com/luci/luci-go/common/tsmon/types"
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
func NewHTTPMonitor(ctx context.Context, client *http.Client, endpoint *url.URL) (Monitor, error) {
	return &httpMonitor{
		client:   client,
		endpoint: endpoint,
	}, nil
}

func (m *httpMonitor) ChunkSize() int {
	return 500
}

func (m *httpMonitor) Send(ctx context.Context, cells []types.Cell) error {
	// Serialize the tsmon cells into protobufs.
	req := &pb.Request{
		Payload: &pb.MetricsPayload{
			MetricsCollection: SerializeCells(cells, clock.Now(ctx)),
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
		return http.NewRequest("POST", m.endpoint.String(), bytes.NewReader(encoded.Bytes()))
	}, func(resp *http.Response) error {
		return resp.Body.Close()
	}, func(resp *http.Response, oErr error) error {
		if resp != nil {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				logging.WithError(err).Errorf(ctx, "Failed to read error response body")
			} else {
				logging.Errorf(ctx, "Monitoring push failed.  Response body: %s", body)
			}
			resp.Body.Close()
		}
		return oErr
	})()
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("bad response status %d from endpoint %s", status, m.endpoint)
	}

	logging.Debugf(ctx, "Sent %d tsmon cells to %s", len(cells), m.endpoint)
	return nil
}

func (m *httpMonitor) Close() error {
	return nil
}
