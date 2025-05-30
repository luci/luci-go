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
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/taskqueue"

	logs_api "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	reg_api "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	services_api "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	reg_impl "go.chromium.org/luci/logdog/appengine/coordinator/endpoints/registration"
	services_impl "go.chromium.org/luci/logdog/appengine/coordinator/endpoints/services"
	logs_impl "go.chromium.org/luci/logdog/appengine/coordinator/flex/logs"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	mem_storage "go.chromium.org/luci/logdog/common/storage/memory"
	logdog_types "go.chromium.org/luci/logdog/common/types"
)

type prefixState struct {
	index  *uint64
	secret []byte

	streamsMu sync.Mutex
	streams   stringset.Set
}

const (
	// Project is the logdog project namespace that you should use with your
	// fakelogs clients.
	Project = "fakelogs-project"
	// Realm is a realm all fake logs end up associated with.
	Realm = "fakelogs-realm"
)

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

	logsServ logs_api.LogsServer
	regServ  reg_api.RegistrationServer
	srvServ  services_api.ServicesServer

	prefixMu sync.Mutex
	prefixes map[string]*prefixState
}

// Get implements logs.Get.
func (c *Client) Get(_ context.Context, in *logs_api.GetRequest, _ ...grpc.CallOption) (*logs_api.GetResponse, error) {
	realIn := proto.Clone(in).(*logs_api.GetRequest)
	realIn.Project = Project
	return c.logsServ.Get(c.ctx, realIn)
}

// Tail implements logs.Tail.
func (c *Client) Tail(_ context.Context, in *logs_api.TailRequest, _ ...grpc.CallOption) (*logs_api.GetResponse, error) {
	realIn := proto.Clone(in).(*logs_api.TailRequest)
	realIn.Project = Project
	return c.logsServ.Tail(c.ctx, realIn)
}

// Query implements logs.Query.
func (c *Client) Query(_ context.Context, in *logs_api.QueryRequest, _ ...grpc.CallOption) (*logs_api.QueryResponse, error) {
	realIn := proto.Clone(in).(*logs_api.QueryRequest)
	realIn.Project = Project
	return c.logsServ.Query(c.ctx, realIn)
}

// OpenTextStream returns a stream for text (line delimited) data.
//
//   - Lines are always delimited with "\n".
//   - Write calls will always have an implied "\n" at the end, if the input
//     data doesn't have one. If you want to do multiple Write calls to affect
//     the same line, you'll need to buffer it yourself.
func (c *Client) OpenTextStream(prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (*Stream, error) {
	return c.open(logpb.StreamType_TEXT, prefix, path, flags...)
}

// OpenDatagramStream returns a stream for datagrams.
//
// Each Write will produce a single complete datagram in the stream.
func (c *Client) OpenDatagramStream(prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (*Stream, error) {
	return c.open(logpb.StreamType_DATAGRAM, prefix, path, flags...)
}

// OpenBinaryStream returns a stream for binary data.
//
// Each Write will append to the binary data like a normal raw file.
func (c *Client) OpenBinaryStream(prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (*Stream, error) {
	return c.open(logpb.StreamType_BINARY, prefix, path, flags...)
}

func prepFlags(streamType logpb.StreamType, name logdog_types.StreamName, flags []*streamproto.Flags) *streamproto.Flags {
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
		rsp, err := c.regServ.RegisterPrefix(c.ctx, &reg_api.RegisterPrefixRequest{
			Project: Project,
			Realm:   Realm,
			Prefix:  prefix,
		})
		if err != nil {
			return nil, errors.Fmt("registering prefix: %w", err)
		}
		idx := uint64(0)
		state = &prefixState{index: &idx, secret: rsp.Secret, streams: stringset.New(0)}
		c.prefixes[prefix] = state
	}

	return state, nil
}

func (c *Client) open(streamType logpb.StreamType, prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (*Stream, error) {
	flg := prepFlags(streamType, path, flags)
	start := clock.Now(c.ctx)
	flg.Timestamp = clockflag.Time(start)
	lsd := flg.Descriptor()
	lsd.Prefix = string(prefix)
	data, err := proto.Marshal(lsd)
	if err != nil {
		panic("error serializing a proto: " + err.Error())
	}

	state, err := c.getPrefixState(string(prefix))
	if err != nil {
		return nil, errors.Fmt("loading prefix state: %w", err)
	}

	state.streamsMu.Lock()
	defer state.streamsMu.Unlock()

	if state.streams.Has(string(path)) {
		return nil, errors.Fmt("duplicate stream: %s/+/%s", prefix, path)
	}

	rsp, err := c.srvServ.RegisterStream(c.ctx, &services_api.RegisterStreamRequest{
		Project:       Project,
		Secret:        state.secret,
		ProtoVersion:  logpb.Version,
		Desc:          data,
		TerminalIndex: -1,
	})
	if err != nil {
		return nil, errors.Fmt("registering stream: %w", err)
	}
	state.streams.Add(string(path))

	return &Stream{
		c: c,

		pth:         lsd.Path(),
		streamType:  streamType,
		prefixIndex: state.index,
		streamID:    rsp.Id,
		secret:      state.secret,
		start:       start,
	}, nil
}

type storageclient struct {
	*mem_storage.Storage
}

// Close is implemented here so that we can return this coordinator.Storage
// client multiple times without worrying about the coordinator closing it.
func (s storageclient) Close() {}

// GetSignedURLs is implemented to fill out the coordinator.Storage interface.
func (s storageclient) GetSignedURLs(context.Context, *coordinator.URLSigningRequest) (*coordinator.URLSigningResponse, error) {
	return nil, errors.New("NOT IMPLEMENTED")
}

// NewClient generates a new fake Client which can be used as a logs.LogsClient,
// and can also have its underlying stream data manipulated by the test.
//
// Functions taking context.Context will ignore it (i.e. they don't expect
// anything in the context).
//
// This client is attached to an in-memory datastore of its own and the real
// coordinator services are attached to that in-memory datastore. That means
// that the Client API SHOULD actually behave identically to the real
// coordinator (since it's the same code). Additionally, the 'Open' methods on
// the Client do the full Prefix/Stream registration process, and so should also
// behave like the real thing.
func NewClient() *Client {
	ctx, env := coordinatorTest.Install()
	env.AddProject(ctx, Project)
	env.ActAsWriter(Project, Realm)
	env.JoinAdmins()
	env.JoinServices()

	ts := taskqueue.GetTestable(ctx)
	ts.CreatePullQueue(services_impl.RawArchiveQueueName(0))

	storage := &mem_storage.Storage{}
	env.Services.ST = func(*coordinator.LogStreamState) (coordinator.SigningStorage, error) {
		return storageclient{storage}, nil
	}
	return &Client{
		ctx: ctx, env: env,

		logsServ: logs_impl.New(),
		regServ:  reg_impl.New(),
		srvServ: services_impl.New(services_impl.ServerSettings{
			NumQueues: 1,
		}),

		storage: storage,

		prefixes: map[string]*prefixState{},
	}
}
