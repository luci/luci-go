// Copyright 2021 The LUCI Authors.
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

// Package accesslog implements an gRPC interceptor that logs calls to a
// BigQuery table.
package accesslog

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/trace"
	codepb "google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/bqlog"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	logpb "go.chromium.org/luci/cipd/api/cipd/v1/logpb"
	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common"
)

func init() {
	bqlog.RegisterSink(bqlog.Sink{
		Prototype: &logpb.AccessLogEntry{},
		Table:     "access",
	})
}

// NewUnaryServerInterceptor returns an interceptor that logs requests.
func NewUnaryServerInterceptor(opts *server.Options) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		if !strings.HasPrefix(info.FullMethod, "/cipd.") {
			return handler(ctx, req)
		}

		start := clock.Now(ctx)
		state := auth.GetState(ctx)

		entry := &logpb.AccessLogEntry{
			Method:         info.FullMethod,
			Timestamp:      start.UnixNano() / 1000,
			CallIdentity:   string(state.User().Identity),
			PeerIdentity:   string(state.PeerIdentity()),
			PeerIp:         state.PeerIP().String(),
			ServiceVersion: opts.ContainerImageID,
			ProcessId:      opts.Hostname,
			RequestId:      trace.SpanContextFromContext(ctx).TraceID().String(),
			AuthDbRev:      authdb.Revision(state.DB()),
		}
		if md, _ := metadata.FromIncomingContext(ctx); len(md["user-agent"]) > 0 {
			entry.UserAgent = strings.Join(md["user-agent"], " ")
		}

		// Extract interesting information from recognized request body types.
		extractFieldsFromRequest(entry, req)

		panicking := true
		defer func() {
			entry.ResponseTimeUsec = clock.Since(ctx, start).Microseconds()
			switch {
			case panicking:
				entry.ResponseCode = "INTERNAL"
				entry.ResponseErr = "panic"
			case err == nil:
				entry.ResponseCode = "OK"
			default:
				entry.ResponseCode = codepb.Code_name[int32(status.Code(err))]
				entry.ResponseErr = err.Error()
			}
			bqlog.Log(ctx, entry)
		}()

		resp, err = handler(ctx, req)
		panicking = false
		return
	}
}

func extractFieldsFromRequest(entry *logpb.AccessLogEntry, req any) {
	if x, ok := req.(interface{ GetPackage() string }); ok {
		entry.Package = x.GetPackage()
	} else if x, ok := req.(interface{ GetPrefix() string }); ok {
		entry.Package = x.GetPrefix()
	}

	if x, ok := req.(interface{ GetInstance() *caspb.ObjectRef }); ok {
		entry.Instance = instanceID(x.GetInstance())
	} else if x, ok := req.(interface{ GetObject() *caspb.ObjectRef }); ok {
		entry.Instance = instanceID(x.GetObject())
	}

	if x, ok := req.(interface{ GetVersion() string }); ok {
		entry.Version = x.GetVersion()
	}

	if x, ok := req.(interface{ GetTags() []*repopb.Tag }); ok {
		entry.Tags = tagList(x.GetTags())
	}

	if x, ok := req.(interface {
		GetMetadata() []*repopb.InstanceMetadata
	}); ok {
		entry.Metadata = metadataKeys(x.GetMetadata())
	}

	// Few one offs that do not follow the pattern.
	switch r := req.(type) {
	case *repopb.Ref:
		entry.Version = r.Name
	case *repopb.DeleteRefRequest:
		entry.Version = r.Name
	case *repopb.ListMetadataRequest:
		entry.Metadata = r.Keys
	case *repopb.DescribeInstanceRequest:
		if r.DescribeRefs {
			entry.Flags = append(entry.Flags, "refs")
		}
		if r.DescribeTags {
			entry.Flags = append(entry.Flags, "tags")
		}
		if r.DescribeProcessors {
			entry.Flags = append(entry.Flags, "processors")
		}
		if r.DescribeMetadata {
			entry.Flags = append(entry.Flags, "metadata")
		}
	}
}

func instanceID(ref *caspb.ObjectRef) string {
	if ref == nil {
		return ""
	}
	if err := common.ValidateObjectRef(ref, common.AnyHash); err != nil {
		return fmt.Sprintf("INVALID:%s", err)
	}
	return common.ObjectRefToInstanceID(ref)
}

func tagList(tags []*repopb.Tag) []string {
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = common.JoinInstanceTag(t)
	}
	return out
}

func metadataKeys(md []*repopb.InstanceMetadata) []string {
	out := make([]string, len(md))
	for i, d := range md {
		out[i] = d.Key
	}
	return out
}
