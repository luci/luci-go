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

	codepb "google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/bqlog"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

func init() {
	bqlog.RegisterSink(bqlog.Sink{
		Prototype: &cipdpb.AccessLogEntry{},
		Table:     "access",
	})
}

// NewUnaryServerInterceptor returns an interceptor that logs requests.
func NewUnaryServerInterceptor(opts *server.Options) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		if !strings.HasPrefix(info.FullMethod, "/cipd.") {
			return handler(ctx, req)
		}

		start := clock.Now(ctx)
		state := auth.GetState(ctx)

		entry := &cipdpb.AccessLogEntry{
			Method:         info.FullMethod,
			Timestamp:      start.UnixNano() / 1000,
			CallIdentity:   string(state.User().Identity),
			PeerIdentity:   string(state.PeerIdentity()),
			PeerIp:         state.PeerIP().String(),
			ServiceVersion: opts.ContainerImageID,
			ProcessId:      opts.Hostname,
			RequestId:      trace.SpanContext(ctx),
			AuthDbRev:      authdb.Revision(state.DB()),
		}
		if md, _ := metadata.FromIncomingContext(ctx); len(md["user-agent"]) > 0 {
			entry.UserAgent = strings.Join(md["user-agent"], " ")
		}

		// Extract interesting information from recognized request body types.
		switch r := req.(type) {
		case *cipdpb.GetObjectURLRequest:
			entry.Instance = instanceID(r.Object)
		case *cipdpb.BeginUploadRequest:
			entry.Instance = instanceID(r.Object)
		case *cipdpb.PrefixRequest:
			entry.Package = r.Prefix
		case *cipdpb.PrefixMetadata:
			entry.Package = r.Prefix
		case *cipdpb.ListPrefixRequest:
			entry.Package = r.Prefix
		case *cipdpb.PackageRequest:
			entry.Package = r.Package
		case *cipdpb.Instance:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
		case *cipdpb.ListInstancesRequest:
			entry.Package = r.Package
		case *cipdpb.SearchInstancesRequest:
			entry.Package = r.Package
			entry.Tags = tagList(r.Tags)
		case *cipdpb.Ref:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
			entry.Version = r.Name
		case *cipdpb.DeleteRefRequest:
			entry.Package = r.Package
			entry.Version = r.Name
		case *cipdpb.ListRefsRequest:
			entry.Package = r.Package
		case *cipdpb.AttachTagsRequest:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
			entry.Tags = tagList(r.Tags)
		case *cipdpb.DetachTagsRequest:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
			entry.Tags = tagList(r.Tags)
		case *cipdpb.AttachMetadataRequest:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
			entry.Metadata = metadataKeys(r.Metadata)
		case *cipdpb.DetachMetadataRequest:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
			entry.Metadata = metadataKeys(r.Metadata)
		case *cipdpb.ListMetadataRequest:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
			entry.Metadata = r.Keys
		case *cipdpb.ResolveVersionRequest:
			entry.Package = r.Package
			entry.Version = r.Version
		case *cipdpb.GetInstanceURLRequest:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
		case *cipdpb.DescribeInstanceRequest:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
		case *cipdpb.DescribeClientRequest:
			entry.Package = r.Package
			entry.Instance = instanceID(r.Instance)
		}

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

func instanceID(ref *cipdpb.ObjectRef) string {
	if ref == nil {
		return ""
	}
	if err := common.ValidateObjectRef(ref, common.AnyHash); err != nil {
		return fmt.Sprintf("INVALID:%s", err)
	}
	return common.ObjectRefToInstanceID(ref)
}

func tagList(tags []*cipdpb.Tag) []string {
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = common.JoinInstanceTag(t)
	}
	return out
}

func metadataKeys(md []*cipdpb.InstanceMetadata) []string {
	out := make([]string, len(md))
	for i, d := range md {
		out[i] = d.Key
	}
	return out
}
