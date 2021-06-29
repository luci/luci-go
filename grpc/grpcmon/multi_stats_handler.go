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

package grpcmon

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// WithMultiStatsHandler returns a DialOption that chains given stats.Handler(s).
// The first handler will be executed first, and then next afterwards.
// nil Handlers are ignored.
func WithMultiStatsHandler(handlers ...stats.Handler) grpc.DialOption {
	mh := &multiStatsHandler{}
	for _, h := range handlers {
		if h == nil {
			continue
		}
		mh.delegates = append(mh.delegates, h)
	}
	switch len(mh.delegates) {
	case 0:
		// Effectively, this unsets the dial option for the stats handler.
		return grpc.WithStatsHandler(nil)
	case 1:
		// Avoid unnecessary delegation layer.
		return grpc.WithStatsHandler(mh.delegates[0])
	}
	return grpc.WithStatsHandler(mh)
}

type multiStatsHandler struct {
	delegates []stats.Handler
}

func (mh *multiStatsHandler) TagRPC(ctx context.Context, i *stats.RPCTagInfo) context.Context {
	for _, d := range mh.delegates {
		ctx = d.TagRPC(ctx, i)
	}
	return ctx
}

func (mh *multiStatsHandler) HandleRPC(ctx context.Context, s stats.RPCStats) {
	for _, d := range mh.delegates {
		d.HandleRPC(ctx, s)
	}
}

func (mh *multiStatsHandler) TagConn(ctx context.Context, i *stats.ConnTagInfo) context.Context {
	for _, d := range mh.delegates {
		ctx = d.TagConn(ctx, i)
	}
	return ctx
}

func (mh *multiStatsHandler) HandleConn(ctx context.Context, s stats.ConnStats) {
	for _, d := range mh.delegates {
		d.HandleConn(ctx, s)
	}
}
