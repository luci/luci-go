// Copyright 2017 The LUCI Authors.
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

package fakelogs

import (
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/errors"
	logs "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	reg "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	services "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	mem_storage "go.chromium.org/luci/logdog/common/storage/memory"
	logdog_types "go.chromium.org/luci/logdog/common/types"
)

type prefixState struct {
	index  *uint64
	secret []byte
}

// Client implements the logs.LogsClient API, and also has some 'reach-around'
// APIs to insert stream data into the backend.
//
// The reach-around APIs are very primitive; they don't simulate butler-side
// constraints (i.e. no prefix secrets, no multi-stream bundling, etc.), however
// from the server-side they should present a sufficient surface to write
// testable server code.
//
// The Open*Stream methods take a singular optional flags object. If provided,
// a copy of this Flags object will be populated with the given logdog path and
// stream type. You can use this to give your streams a content type, tags, etc.
type Client struct {
	ctx context.Context
	env *coordinatorTest.Environment

	storage *mem_storage.Storage

	logsServ logs.LogsServer
	regServ  reg.RegistrationServer
	srvServ  services.ServicesServer

	prefixMu sync.Mutex
	prefixes map[string]*prefixState
}

// Get implements logs.Get.
func (c *Client) Get(_ context.Context, in *logs.GetRequest, _ ...grpc.CallOption) (*logs.GetResponse, error) {
	realIn := *in
	realIn.Project = coordinatorTest.AllAccessProject
	return c.logsServ.Get(c.ctx, &realIn)
}

// Tail implements logs.Tail.
func (c *Client) Tail(_ context.Context, in *logs.TailRequest, _ ...grpc.CallOption) (*logs.GetResponse, error) {
	realIn := *in
	realIn.Project = coordinatorTest.AllAccessProject
	return c.logsServ.Tail(c.ctx, &realIn)
}

// Query implements logs.Query.
func (c *Client) Query(_ context.Context, in *logs.QueryRequest, _ ...grpc.CallOption) (*logs.QueryResponse, error) {
	realIn := *in
	realIn.Project = coordinatorTest.AllAccessProject
	return c.logsServ.Query(c.ctx, &realIn)
}

// OpenTextStream returns a stream for text (line delimited) data.
//
//  - Lines are always delimited with "\n".
//  - Write calls will always have an implied "\n" at the end, if the input
//    data doesn't have one. If you want to do multiple Write calls to affect
//    the same line, you'll need to buffer it yourself.
func (c *Client) OpenTextStream(prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (*Stream, error) {
	return c.open(logpb.StreamType_TEXT, path, prefix, flags...)
}

// OpenDatagramStream returns a stream for datagrams.
//
// Each Write will produce a single complete datagram in the stream.
func (c *Client) OpenDatagramStream(prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (*Stream, error) {
	return c.open(logpb.StreamType_DATAGRAM, path, prefix, flags...)
}

// OpenBinaryStream returns a stream for binary data.
//
// Each Write will append to the binary data like a normal raw file.
func (c *Client) OpenBinaryStream(prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (*Stream, error) {
	return c.open(logpb.StreamType_BINARY, path, prefix, flags...)
}

func prepFlags(streamType logpb.StreamType, name logdog_types.StreamName, flags ...*streamproto.Flags) *streamproto.Flags {
	if _, ok := logpb.StreamType_name[int32(streamType)]; !ok {
		panic("unknown logpb.StreamType: " + streamType.String())
	}

	typ := streamproto.StreamType(streamType)

	switch len(flags) {
	case 0:
		return &streamproto.Flags{
			Name: streamproto.StreamNameFlag(name),
			Type: typ,
		}

	case 1:
		newFlags := *flags[0]
		newFlags.Name = streamproto.StreamNameFlag(name)
		newFlags.Type = typ
		return &newFlags

	default:
		panic(fmt.Sprintf("`flags...` must can exactly 0 or 1 value, got %d", len(flags)))
	}
}

func (c *Client) getPrefixState(prefix string) (*prefixState, error) {
	c.prefixMu.Lock()
	defer c.prefixMu.Unlock()

	state, seenPrefix := c.prefixes[prefix]
	if !seenPrefix {
		rsp, err := c.regServ.RegisterPrefix(c.ctx, &reg.RegisterPrefixRequest{
			Project: coordinatorTest.AllAccessProject,
			Prefix:  prefix,
		})
		if err != nil {
			return nil, errors.Annotate(err, "registering prefix").Err()
		}
		idx := uint64(0)
		state = &prefixState{&idx, rsp.Secret}
		c.prefixes[prefix] = state
	}

	return state, nil
}

func (c *Client) open(streamType logpb.StreamType, prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (*Stream, error) {
	flg := prepFlags(streamType, path, flags...)
	start := clock.Now(c.ctx)
	flg.Timestamp = clockflag.Time(start)
	lsd := flg.Properties().LogStreamDescriptor
	lsd.Prefix = string(prefix)
	data, err := proto.Marshal(lsd)
	if err != nil {
		panic("error serializing a proto: " + err.Error())
	}

	state, err := c.getPrefixState(string(prefix))
	if err != nil {
		return nil, errors.Annotate(err, "loading prefix state").Err()
	}

	rsp, err := c.srvServ.RegisterStream(c.ctx, &services.RegisterStreamRequest{
		Project:       coordinatorTest.AllAccessProject,
		Secret:        state.secret,
		ProtoVersion:  logpb.Version,
		Desc:          data,
		TerminalIndex: -1,
	})
	if err != nil {
		return nil, errors.Annotate(err, "registering stream").Err()
	}

	return &Stream{
		ctx:         c.ctx,
		storage:     c.storage,
		srvSrv:      c.srvServ,
		pth:         lsd.Path(),
		prefixIndex: state.index,
		streamID:    rsp.Id,
		secret:      state.secret,
		start:       start,
	}, nil
}
