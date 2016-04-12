// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/client/tokenclient"
	"github.com/luci/luci-go/common/api/tokenserver/v1"
	"github.com/luci/luci-go/common/logging/memlogger"
)

// FailureReason describes why tokend run failed.
type FailureReason string

// Some knows error reasons.
//
// See also FailureReasonFromRPCError for error reasons generated from status
// codes.
const (
	FailureCantReadKey      FailureReason = "CANT_READ_KEY"
	FailureMalformedReponse FailureReason = "MALFORMED_RESPONSE"
	FailureUnknownRPCError  FailureReason = "UNKNOWN_RPC_ERROR"
	FailureCantSaveToken    FailureReason = "CANT_SAVE_TOKEN"
)

// FailureReasonFromRPCError transform MintToken error into a failure reason.
func FailureReasonFromRPCError(err error) FailureReason {
	if err == nil {
		return ""
	}
	if details, ok := err.(tokenclient.RPCError); ok {
		if details.GrpcCode != codes.OK {
			return FailureReason(fmt.Sprintf("GRPC_ERROR_%d", details.GrpcCode))
		}
		return FailureReason(fmt.Sprintf("MINT_TOKEN_ERROR_%s", details.ErrorCode))
	}
	return FailureUnknownRPCError
}

// UpdateReason describes why tokend attempts to update the token.
type UpdateReason string

// All known reasons for starting token refresh procedure.
const (
	UpdateReasonNewToken         UpdateReason = "NEW_TOKEN"
	UpdateReasonExpiration       UpdateReason = "TOKEN_EXPIRES"
	UpdateReasonParametersChange UpdateReason = "PARAMS_CHANGE"
	UpdateReasonForceRefresh     UpdateReason = "FORCE_REFRESH"
)

// StatusFile gathers information about tokend run and dumps it to disk.
//
// It is picked up by monitoring harness later.
type StatusFile struct {
	Path string // path to put the status file to or "" to skip saving

	Version           string                 // major version of the tokend executable
	Started           time.Time              // when the process started
	Finished          time.Time              // when the process finished
	UpdateReason      UpdateReason           // why tokend attempts to update the token
	FailureReason     FailureReason          // why token update failed or empty string on success
	FailureError      error                  // immediate error that caused the failure
	LogDump           string                 // entire update log
	MintTokenDuration time.Duration          // how long RPC call lasted (with all retries)
	LastToken         *tokenserver.TokenFile // last known token (possibly refreshed)
}

// statusFileJSON is how status file looks on disk.
type statusFileJSON struct {
	TokendVersion       string `json:"tokend_version"`
	StartedTS           int64  `json:"started_ts"`
	Success             bool   `json:"success"`
	TotalDurationMS     int64  `json:"total_duration_ms,omitempty"`
	MintTokenDurationMS int64  `json:"mint_token_duration_ms,omitempty"`
	UpdateReason        string `json:"update_reason,omitempty"`
	FailureReason       string `json:"failure_reason,omitempty"`
	FailureError        string `json:"failure_error,omitempty"`
	LogDump             string `json:"log_dump"`
	TokenLastUpdateTS   int64  `json:"token_last_update_ts,omitempty"`
	TokenNextUpdateTS   int64  `json:"token_next_update_ts,omitempty"`
	TokenExpiryTS       int64  `json:"token_expiry_ts,omitempty"`
}

// Close flushes the status to disk.
//
// Does nothing is Path is empty string.
func (s *StatusFile) Close(l *memlogger.MemLogger) error {
	if s.Path == "" {
		return nil
	}

	buf := bytes.Buffer{}
	l.Dump(&buf)
	s.LogDump = buf.String()

	status := &statusFileJSON{
		TokendVersion:       s.Version,
		StartedTS:           s.Started.Unix(),
		Success:             s.FailureReason == "",
		TotalDurationMS:     s.Finished.Sub(s.Started).Nanoseconds() / 1000000,
		MintTokenDurationMS: s.MintTokenDuration.Nanoseconds() / 1000000,
		UpdateReason:        string(s.UpdateReason),
		FailureReason:       string(s.FailureReason),
		LogDump:             s.LogDump,
	}
	if s.FailureError != nil {
		status.FailureError = s.FailureError.Error()
	}
	if s.LastToken != nil {
		status.TokenLastUpdateTS = s.LastToken.LastUpdate
		status.TokenNextUpdateTS = s.LastToken.NextUpdate
		status.TokenExpiryTS = s.LastToken.Expiry
	}

	blob, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(s.Path, blob, 0644)
}
