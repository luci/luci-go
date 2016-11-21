// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package logservice provides a client which can be used to to collect and send batches of logs to the eventlog service.
package logservice

import (
	"bytes"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"

	logpb "github.com/luci/luci-go/common/eventlog/proto"
	"golang.org/x/net/context"
)

// Client sends event logs to the eventlog service.
type Client struct {
	HTTPClient *http.Client
	serverAddr string
	logSource  string
}

// NewClient constructs a new Client.
// Users must call Close when the Client is no longer needed.
func NewClient(serverAddr, logSourceName string) *Client {
	return &Client{
		serverAddr: serverAddr,
		logSource:  logSourceName,
	}
}

// TODO(mcgreevy): support bundling log requests.

// LogSync synchronously logs events to the eventlog service.
// The EventTime in each event must have been obtained from time.Now.
func (c *Client) LogSync(ctx context.Context, events []*logpb.LogRequestLite_LogEventLite) error {
	// TODO(mcgreevy): consider supporting custom clocks.
	log := &logpb.LogRequestLite{
		RequestTimeMs: proto.Int64(time.Now().UnixNano() / 1e6),
		LogSourceName: &c.logSource,
		LogEvent:      events,
	}

	buf, err := proto.Marshal(log)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.serverAddr, bytes.NewReader(buf))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req = req.WithContext(ctx)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

// Close flushes any pending logs and releases any resources held by the client.
// Close should be called when the client is no longer needed.
func (c *Client) Close() error {
	// We will need this later, but for now it's a no-op.
	return nil
}
