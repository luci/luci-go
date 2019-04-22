// Copyright 2019 The LUCI Authors.
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

package main

import (
	"io/ioutil"
	"os"

	"go.chromium.org/luci/common/clock/testclock"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var now = testclock.TestRecentTimeUTC

func initExecutable() (initBuild *pb.Build, client streamclient.Client, buildStream streamclient.Stream) {
	stdin, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	initBuild = &pb.Build{}
	if err := proto.Unmarshal(stdin, initBuild); err != nil {
		panic(err)
	}

	bootstrapped, err := bootstrap.Get()
	if err != nil {
		panic(err)
	}

	client = bootstrapped.Client
	buildStream, err = client.NewStream(streamproto.Flags{
		Name:        streamproto.StreamNameFlag("build.proto"),
		Type:        streamproto.StreamType(logpb.StreamType_DATAGRAM),
		ContentType: protoutil.BuildMediaType,
		Timestamp:   clockflag.Time(now),
	})
	if err != nil {
		panic(err)
	}
	return
}

func writeBuild(s streamclient.Stream, build *pb.Build) {
	buf, err := proto.Marshal(build)
	if err != nil {
		panic(err)
	}
	if err := s.WriteDatagram(buf); err != nil {
		panic(err)
	}
}
