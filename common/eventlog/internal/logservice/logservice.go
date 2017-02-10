// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package logservice provides a client which can be used to to collect and send batches of logs to the eventlog service.
package logservice

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	logpb "github.com/luci/luci-go/common/eventlog/proto"
)

// Logger sends event logs to the eventlog service.
type Logger struct {
	HTTPClient *http.Client

	serverAddr string
	logSource  string
}

// NewLogger constructs a new Client.
// Users must call Close when the Client is no longer needed.
func NewLogger(serverAddr, logSourceName string) *Logger {
	return &Logger{
		serverAddr: serverAddr,
		logSource:  logSourceName,
	}
}

// retryError is an error for an operation that should be retried.
type retryError struct {
	error
}

// LogSync synchronously logs events to the eventlog service.
// The EventTime in each event must have been obtained from time.Now.
// The returned error can be supplied to ShouldRetry to determine whether the operation should be retried.
func (l *Logger) LogSync(ctx context.Context, events ...*logpb.LogRequestLite_LogEventLite) error {
	// TODO(mcgreevy): consider supporting custom clocks.
	log := &logpb.LogRequestLite{
		RequestTimeMs: proto.Int64(time.Now().UnixNano() / 1e6),
		LogSourceName: &l.logSource,
		LogEvent:      events,
	}

	buf, err := proto.Marshal(log)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", l.serverAddr, bytes.NewReader(buf))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req = req.WithContext(ctx)
	resp, err := l.HTTPClient.Do(req)
	if err != nil {
		return retryError{err}
	}
	defer resp.Body.Close()

	sc := resp.StatusCode
	switch {
	case sc == 200:
		return nil
	case sc == 400:
		// malformed; this should not happen.
		return fmt.Errorf("eventlog: malformed request: %v", req)
	case sc == 401:
		return retryError{fmt.Errorf("eventlog: auth failed: %v", req)}
	case sc >= 500 && sc < 600:
		return retryError{fmt.Errorf("eventlog: req failed: %v", req)}
	default:
		return fmt.Errorf("eventlog: unexpected status code: %v for request: %v", sc, req)
	}
}

// ShouldRetry reports whether a LogSync call (which returned err) should be retried.
func ShouldRetry(err error) bool {
	_, ok := err.(retryError)
	return ok
}
