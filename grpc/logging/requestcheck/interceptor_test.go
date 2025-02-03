// Copyright 2024 The LUCI Authors.
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

package requestcheck

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/grpc/logging/requestcheck/testproto"
)

func TestLog(t *testing.T) {
	t.Run(`nonsense`, func(t *testing.T) {
		t.Parallel()
		ctx := memlogger.Use(context.Background())
		ml := logging.Get(ctx).(*memlogger.MemLogger)

		dl := makeDeprecationLogger(nil)

		dl.log(ctx, "I am not a proto message")

		check.Loosely(t, ml.Messages(), should.BeEmpty)
	})

	t.Run(`nothing set`, func(t *testing.T) {
		t.Parallel()
		ctx := memlogger.Use(context.Background())
		ml := logging.Get(ctx).(*memlogger.MemLogger)

		dl := makeDeprecationLogger([]protoreflect.MessageDescriptor{
			(*testproto.UnaryRequest)(nil).ProtoReflect().Descriptor(),
		})

		dl.log(ctx, &testproto.UnaryRequest{
			Ignore: "stuff",
		})

		check.Loosely(t, ml.Messages(), should.BeEmpty)
	})

	t.Run(`deprecated field set`, func(t *testing.T) {
		t.Parallel()
		ctx := memlogger.Use(context.Background())
		ml := logging.Get(ctx).(*memlogger.MemLogger)

		dl := makeDeprecationLogger([]protoreflect.MessageDescriptor{
			(*testproto.UnaryRequest)(nil).ProtoReflect().Descriptor(),
		})

		dl.log(ctx, &testproto.UnaryRequest{
			Ignore:   "stuff",
			BadField: true,
		})

		check.Loosely(t, ml.Messages(), should.Match([]memlogger.LogEntry{
			{
				Level: logging.Warning,
				Msg:   `go.chromium.org.luci.grpc.logging.requestcheck.testproto.UnaryRequest: deprecated fields set: [".bad_field"]`,
				Data: map[string]any{
					"activity": "requestcheck",
					"fields":   []string{".bad_field"},
					"proto":    "go.chromium.org.luci.grpc.logging.requestcheck.testproto.UnaryRequest",
				},
				CallDepth: 2,
			},
		}))
	})
}

func TestLimit(t *testing.T) {
	origInterval := minInterval

	defer func() {
		minInterval = origInterval
	}()
	minInterval = time.Second

	ctx := memlogger.Use(context.Background())
	ml := logging.Get(ctx).(*memlogger.MemLogger)

	dl := makeDeprecationLogger([]protoreflect.MessageDescriptor{
		(*testproto.UnaryRequest)(nil).ProtoReflect().Descriptor(),
	})

	req := &testproto.UnaryRequest{
		BadField: true,
	}

	start := time.Now()
	for range 200 {
		dl.log(ctx, req)
	}
	elapsed := time.Since(start)
	t.Log("elapsed: ", elapsed)

	if elapsed > time.Second {
		t.Skip("skipping to avoid possible flake")
	}

	check.Loosely(t, ml.Messages(), should.Match([]memlogger.LogEntry{
		{
			Level: logging.Warning,
			Msg:   `go.chromium.org.luci.grpc.logging.requestcheck.testproto.UnaryRequest: deprecated fields set: [".bad_field"]`,
			Data: map[string]any{
				"activity": "requestcheck",
				"fields":   []string{".bad_field"},
				"proto":    "go.chromium.org.luci.grpc.logging.requestcheck.testproto.UnaryRequest",
			},
			CallDepth: 2,
		},
	}))
	ml.Reset()

	toSleep := (time.Second - elapsed) + (20 * time.Millisecond)
	t.Log("toSleep: ", toSleep)
	time.Sleep(toSleep)

	dl.log(ctx, req)
	check.Loosely(t, ml.Messages(), should.Match([]memlogger.LogEntry{
		{
			Level: logging.Warning,
			Msg:   `go.chromium.org.luci.grpc.logging.requestcheck.testproto.UnaryRequest (ignored 199): deprecated fields set: [".bad_field"]`,
			Data: map[string]any{
				"activity": "requestcheck",
				"fields":   []string{".bad_field"},
				"proto":    "go.chromium.org.luci.grpc.logging.requestcheck.testproto.UnaryRequest",
			},
			CallDepth: 2,
		},
	}))
}

type svcDescCatcher struct {
	svcDesc *grpc.ServiceDesc
}

var _ grpc.ServiceRegistrar = (*svcDescCatcher)(nil)

func (s *svcDescCatcher) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.svcDesc = desc
}

func TestExtractDescriptors(t *testing.T) {
	var catcher svcDescCatcher
	testproto.RegisterExampleServiceServer(&catcher, nil)

	ctx := memlogger.Use(context.Background())
	ml := logging.Get(ctx).(*memlogger.MemLogger)

	msgDescs := extractDescriptors(ctx, []*grpc.ServiceDesc{catcher.svcDesc})

	// check that there were no logs
	if !check.Loosely(t, ml.Messages(), should.BeEmpty) {
		ml.LogTo(t)
		t.Fail()
	}

	names := stringset.New(len(msgDescs))
	for _, msgD := range msgDescs {
		names.Add(string(msgD.FullName()))
	}
	// check no duplicates
	assert.Loosely(t, names, should.HaveLength(len(msgDescs)))
	assert.That(t, names, should.Match(stringset.NewFromSlice(
		"go.chromium.org.luci.grpc.logging.requestcheck.testproto.BidirectionalStreamRequest",
		"go.chromium.org.luci.grpc.logging.requestcheck.testproto.ClientStreamRequest",
		"go.chromium.org.luci.grpc.logging.requestcheck.testproto.ServerStreamRequest",
		"go.chromium.org.luci.grpc.logging.requestcheck.testproto.UnaryRequest",
	)))
}
