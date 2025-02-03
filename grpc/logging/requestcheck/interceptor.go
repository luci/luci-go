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

// Package requestcheck has protobuf logging functions.
//
// Currently this only has a set of grpc interceptors which will log when the
// client populates deprecated fields (i.e. fields in the request proto which
// were marked with the field option `[deprecated = true]`).
//
// The implementation uses the go.chromium.org/luci/common/proto/protowalk
// library, so in the average case this will only touch O(F) fields in the
// message where F are fields marked as deprecated. This means that the
// per-request overhead for request messages with no deprecated field option is
// ~zero.
//
// To further cut down on spam, this is also rate-limits the deprecated field
// detection to no more than once per 10s, per (user agent x request type).
// This rate limit only applies to this process, though, so if your service has
// many different processes you may see a burst of logged warnings.
package requestcheck

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
)

var minInterval = 10 * time.Second

var protoMessageType = reflect.TypeFor[proto.Message]()

type throttleKey struct {
	userAgent string
	msgD      protoreflect.MessageDescriptor
}

type throttleEntry struct {
	mu sync.Mutex

	// zero or a time when we did our last log.
	lastLog time.Time
	ignored int64
}

type deprecationLogger struct {
	walkers map[protoreflect.MessageDescriptor]*protowalk.DynamicWalker

	throttles *lru.Cache[throttleKey, *throttleEntry]
}

func (l *deprecationLogger) log(ctx context.Context, req any) {
	msg, ok := req.(proto.Message)
	if !ok {
		return
	}

	agent := "<UNKNOWN>"
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		agentValues := md.Get("user-agent")
		if len(agentValues) > 0 {
			agent = agentValues[0]
		}
	}
	msgD := msg.ProtoReflect().Descriptor()
	walker, ok := l.walkers[msgD]
	if !ok {
		return
	}

	// NOTE: We expect this to take between 10 and 4000ns, scaling with the number
	// of fields marked 'deprecated' in the message.
	results := walker.MustExecute(msg)[0]
	if len(results) == 0 {
		// Nothing to log.
		return
	}

	shouldLog, ignored := func() (shouldLog bool, ignored int64) {
		now := clock.Now(ctx)
		key := throttleKey{agent, msgD}
		throttle, _ := l.throttles.GetOrCreate(ctx, key, func() (v *throttleEntry, exp time.Duration, err error) {
			// We want to keep the entry for 2x the minInterval - if users are calling
			// less frequently then this, then we'll always log them anyway. It's
			// possible we'll lose some amount of `ignored` this way, but that's fine.
			return &throttleEntry{}, 2 * minInterval, nil
		})

		throttle.mu.Lock()
		defer throttle.mu.Unlock()

		if throttle.lastLog.IsZero() || now.Sub(throttle.lastLog) > minInterval {
			// something to log, set lastLog to now
			throttle.lastLog = now
			ignored = throttle.ignored
			throttle.ignored = 0
			return true, ignored
		}

		// we're throttled, so bump ignored and return
		throttle.ignored += 1
		return false, 0
	}()

	if !shouldLog {
		return
	}

	ignoreMsg := ""
	if ignored > 0 {
		ignoreMsg = fmt.Sprintf(" (ignored %d)", ignored)
	}

	fields := make([]string, len(results))
	for i, rslt := range results {
		fields[i] = rslt.Path.String()
	}

	logging.Fields{
		"activity": "requestcheck",
		"proto":    string(msgD.FullName()),
		"fields":   fields,
	}.Warningf(ctx, "%s%s: deprecated fields set: %q", msgD.FullName(), ignoreMsg, fields)
}

func makeDeprecationLogger(msgDescriptors []protoreflect.MessageDescriptor) *deprecationLogger {
	l := &deprecationLogger{
		make(map[protoreflect.MessageDescriptor]*protowalk.DynamicWalker, len(msgDescriptors)),
		lru.New[throttleKey, *throttleEntry](100 * len(msgDescriptors)),
	}
	for _, desc := range msgDescriptors {
		if _, done := l.walkers[desc]; !done {
			l.walkers[desc] = protowalk.NewDynamicWalker(desc, protowalk.DeprecatedProcessor{})
		}
	}
	return l
}

type streamWrapper struct {
	grpc.ServerStream
	l *deprecationLogger
}

func (s streamWrapper) RecvMsg(m any) error {
	err := s.ServerStream.RecvMsg(m)
	if err != nil {
		return err
	}
	s.l.log(s.Context(), m)
	return nil
}

// extractDescriptors extracts the request proto.Message descriptors from a set
// of grpc.ServiceDesc's.
//
// Processing errors are logged via `ctx`.
//
// This supports unary and streaming RPCs of all known generated code flavors.
func extractDescriptors(ctx context.Context, serviceDescs []*grpc.ServiceDesc) []protoreflect.MessageDescriptor {
	var msgDescs []protoreflect.MessageDescriptor

	// NOTE: *grpc.ServiceDesc.HandlerType is a pointer to the generated interface
	// type. In particular, this is NOT a user-provided instance of the service,
	// and so cannot have methods on it other than the ones derived from the proto
	// file's `service` block.
	//
	// We have unittests which use the protos in ./testproto/ to ensure the below
	// code works as intended. If you start seeing log messages from below, it
	// likely means that gRPC code generation changed and so the processing loop
	// below needs to be rewritten.

	const funcName = "requestcheck.DeprecationLoggerInterceptors"

	// firstArgMethod gets the first real argument after looking up methodName in
	// serviceIface.
	//
	// If expectContext is false, this returns the 0th argument, otherwise returns
	// the 1st argument.
	firstArgOfMethod := func(category, methodName string, expectContext bool, serviceIface reflect.Type) reflect.Type {
		mType, ok := serviceIface.MethodByName(methodName)
		if !ok {
			logging.Errorf(ctx, funcName+": %s %q is missing.", category, methodName)
			return nil
		}
		idx := 0
		if expectContext {
			idx++
		}

		if mType.Type.NumIn() <= idx {
			logging.Errorf(ctx, funcName+": %s %q doesn't have enough args (expected >%d).", category, methodName, idx)
			return nil
		}
		return mType.Type.In(idx)
	}

	for _, srvDesc := range serviceDescs {
		typ := reflect.TypeOf(srvDesc.HandlerType).Elem()
		if typ.Kind() != reflect.Interface {
			logging.Errorf(ctx, funcName+": %q HandlerType is not an *interface.", srvDesc.ServiceName)
			continue
		}

		type firstArgProtoMethod struct {
			name          string
			category      string
			expectContext bool
		}
		firstArgProtoMethods := make([]firstArgProtoMethod, 0, len(srvDesc.Methods))
		for _, unaryMethod := range srvDesc.Methods {
			firstArgProtoMethods = append(firstArgProtoMethods, firstArgProtoMethod{
				name:          unaryMethod.MethodName,
				category:      "unary method",
				expectContext: true,
			})
		}
		for _, streamingMethod := range srvDesc.Streams {
			// We skip streaming RPCs where the client streams to us - that is,
			// that the service method's first argument will be a streaming server of
			// some type.
			if streamingMethod.ClientStreams {
				continue
			}
			firstArgProtoMethods = append(firstArgProtoMethods, firstArgProtoMethod{
				name:          streamingMethod.StreamName,
				category:      "server streaming method",
				expectContext: false,
			})
		}

		// Methods which just take a request proto directly.
		//
		// These will be either of:
		//
		//   ServerStream(REQUEST_PROTO, ...) error
		//   Unary(context.Context, REQUEST_PROTO) (..., error)
		for _, firstArgMethod := range firstArgProtoMethods {
			arg := firstArgOfMethod(firstArgMethod.category, firstArgMethod.name, firstArgMethod.expectContext, typ)
			if arg == nil {
				continue
			}
			if !arg.Implements(protoMessageType) {
				logging.Errorf(ctx, funcName+": %s %q: arg 0: %s does not implement proto.Message.", firstArgMethod.category, firstArgMethod.name, arg)
				continue
			}
			msgDescs = append(msgDescs, reflect.Zero(arg).Interface().(proto.Message).ProtoReflect().Descriptor())
		}

		// Streaming methods
		//
		// This loop processes streaming methods where ClientStreams is true,
		// meaning that we can see:
		//
		//   BidirectionalStream(grpc.BidiStreamingServer[...]) error
		//   ClientStream(grpc.ClientStreamingServer[...]) error
		//
		// In both cases, the first argument is an interface with:
		//
		//   Recv() (REQUEST_PROTO, ...)
		for _, streamingMethod := range srvDesc.Streams {
			const category = "client streaming method"
			if !streamingMethod.ClientStreams {
				// These were handled in the above `firstArgMethod` loop. These are
				// streaming RPCs where the client just sends a singular message to
				// start the stream and cannot push messages to the server (and thus the
				// server cannot Recv() them).
				continue
			}

			sName := streamingMethod.StreamName
			arg := firstArgOfMethod(category, sName, false, typ)
			if arg == nil {
				continue
			}
			recvMethod, ok := arg.MethodByName("Recv")
			if !ok {
				logging.Errorf(ctx, funcName+": "+category+" %q: arg 0: %s does not have Recv method.", sName, arg)
				continue
			}

			recvType := recvMethod.Type
			if recvType.NumOut() == 0 {
				logging.Errorf(ctx, funcName+": "+category+" %q: arg 0: %s.Recv does not return values.", sName, arg)
				continue
			}
			arg = recvType.Out(0)
			if !arg.Implements(protoMessageType) {
				logging.Errorf(ctx, funcName+": "+category+" %q: arg 0: %s.Recv does not return proto.Message.", sName, arg)
				continue
			}

			msgDescs = append(msgDescs, reflect.Zero(arg).Interface().(proto.Message).ProtoReflect().Descriptor())
		}
	}
	return msgDescs
}

// DeprecationLoggerInterceptors returns a pair of unary and stream interceptors
// which will walk all incoming request protos and log (in a rate limited way)
// when incoming requests have non-default values for fields marked with the
// `deprecated = true` field option.
func DeprecationLoggerInterceptors(ctx context.Context, serviceDescs []*grpc.ServiceDesc) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor) {
	l := makeDeprecationLogger(extractDescriptors(ctx, serviceDescs))

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			l.log(ctx, req)
			return handler(ctx, req)
		}, func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, streamWrapper{ss, l})
		}
}
