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
	"sync"

	"github.com/golang/protobuf/proto"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/data/stringset"
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
	seenStreams stringset.Set
	index       *uint64
	secret      []byte
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

	mutex sync.Mutex

	prefixes map[string]*prefixState
}

// Get implements logs.Get.
func (c *Client) Get(_ context.Context, in *logs.GetRequest, _ ...grpc.CallOption) (*logs.GetResponse, error) {
	return c.logsServ.Get(c.ctx, in)
}

// Tail implements logs.Tail.
func (c *Client) Tail(_ context.Context, in *logs.TailRequest, _ ...grpc.CallOption) (*logs.GetResponse, error) {
	return c.logsServ.Tail(c.ctx, in)
}

// Query implements logs.Query.
func (c *Client) Query(_ context.Context, in *logs.QueryRequest, _ ...grpc.CallOption) (*logs.QueryResponse, error) {
	return c.logsServ.Query(c.ctx, in)
}

// OpenTextStream returns a stream for text (line delimited) data.
//
// Lines are always delimited with "\n".
// Write calls will always have an implied "\n" at the end, if the input data
//   doesn't have one. If you want to do multiple Write calls to affect the
//   same line, you'll need to buffer it yourself.
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

func prepFlags(streamType logpb.StreamType, name logdog_types.StreamName, flags ...*streamproto.Flags) (*streamproto.Flags, error) {
	if _, ok := logpb.StreamType_name[int32(streamType)]; !ok {
		return nil, errors.Reason("unknown logpb.StreamType: %s", streamType).Err()
	}

	if err := name.Validate(); err != nil {
		return nil, errors.Annotate(err, "bad stream name").Err()
	}

	typ := streamproto.StreamType(streamType)

	switch len(flags) {
	case 0:
		return &streamproto.Flags{
			Name:        streamproto.StreamNameFlag(name),
			Type:        typ,
			ContentType: string(typ.DefaultContentType()),
		}, nil

	case 1:
		newFlags := *flags[0]
		newFlags.Name = streamproto.StreamNameFlag(name)
		newFlags.Type = typ
		if newFlags.ContentType == "" {
			newFlags.ContentType = string(typ.DefaultContentType())
		}
		return &newFlags, nil

	default:
		return nil, errors.Reason("`flags...` must can exactly 0 or 1 value, got %d", len(flags)).Err()
	}
}

func (c *Client) open(streamType logpb.StreamType, prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (*Stream, error) {
	if err := prefix.Validate(); err != nil {
		return nil, errors.Annotate(err, "bad stream prefix").Err()
	}

	flg, err := prepFlags(streamType, path, flags...)
	if err != nil {
		return nil, err
	}

	full := prefix.Join(logdog_types.StreamName(flg.Name))

	prefixS, pathS := string(prefix), string(path)

	c.mutex.Lock()
	defer c.mutex.Unlock()

	state, seenPrefix := c.prefixes[prefixS]
	if !seenPrefix {
		if c.prefixes == nil {
			c.prefixes = map[string]*prefixState{}
		}
		rsp, err := c.regServ.RegisterPrefix(c.ctx, &reg.RegisterPrefixRequest{
			Project: coordinatorTest.AllAccessProject,
			Prefix:  prefixS,
		})
		if err != nil {
			panic("inconsistent state: failed to register prefix: " + err.Error())
		}
		idx := uint64(0)
		state = &prefixState{stringset.New(0), &idx, rsp.Secret}
		c.prefixes[prefixS] = state
	}
	seen := state.seenStreams.Has(pathS)
	if !seen {
		state.seenStreams.Add(pathS)
	}
	if seen {
		return nil, errors.Reason("cannot open the same stream more than once: %q", full).Err()
	}

	data, err := proto.Marshal(flg.Properties().LogStreamDescriptor)
	if err != nil {
		panic("error serializing a proto: " + err.Error())
	}

	rsp, err := c.srvServ.RegisterStream(c.ctx, &services.RegisterStreamRequest{
		Project:       coordinatorTest.AllAccessProject,
		Secret:        state.secret,
		ProtoVersion:  logpb.Version,
		Desc:          data,
		TerminalIndex: -1,
	})
	if err != nil {
		panic("inconsistent state: failed to register stream: " + err.Error())
	}

	return &Stream{
		ctx:         c.ctx,
		storage:     c.storage,
		srvSrv:      c.srvServ,
		pth:         full,
		prefixIndex: state.index,
		streamID:    rsp.Id,
		secret:      state.secret,
	}, nil
}
