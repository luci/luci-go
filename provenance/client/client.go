// Copyright 2022 The LUCI Authors.
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

package client

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	snooperpb "go.chromium.org/luci/provenance/api/snooperpb/v1"
)

// Default timeout for RPC calls to Spike
var timeout = 10 * time.Second

var _ snooperpb.SelfReportClient = (*client)(nil)

type client struct {
	client snooperpb.SelfReportClient
}

// MakeProvenanceClient creates a client to interact with Self-report server.
func MakeProvenanceClient(ctx context.Context, addr string) (*client, error) {
	parsedAddr, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid server address, got: %s, err: %w", addr, err)
	}

	if parsedAddr.Scheme != "http" {
		return nil, fmt.Errorf("invalid address url, expecting http, got: %v", parsedAddr.Scheme)
	}

	conn, err := grpc.NewClient(parsedAddr.Host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to open grpc conn: %w", err)
	}

	return &client{
		client: snooperpb.NewSelfReportClient(conn),
	}, nil
}

// ReportCipd reports cipd package via provenance local server.
func (c *client) ReportCipd(ctx context.Context, in *snooperpb.ReportCipdRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.client.ReportCipd(ctx, in, opts...)
}

// ReportGit reports git checkout via provenance local server.
func (c *client) ReportGit(ctx context.Context, in *snooperpb.ReportGitRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.client.ReportGit(ctx, in, opts...)
}

// ReportGit reports gcs download via provenance local server.
func (c *client) ReportGcs(ctx context.Context, in *snooperpb.ReportGcsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.client.ReportGcs(ctx, in, opts...)
}

// ReportTaskStage reports task stage via provenance local server.
func (c *client) ReportTaskStage(ctx context.Context, in *snooperpb.ReportTaskStageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.client.ReportTaskStage(ctx, in, opts...)
}

// ReportPID reports task pid via provenance local server.
func (c *client) ReportPID(ctx context.Context, in *snooperpb.ReportPIDRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.client.ReportPID(ctx, in, opts...)
}

// ReportArtifactDigest reports artifact digest via provenance local server.
func (c *client) ReportArtifactDigest(ctx context.Context, in *snooperpb.ReportArtifactDigestRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.client.ReportArtifactDigest(ctx, in, opts...)
}
