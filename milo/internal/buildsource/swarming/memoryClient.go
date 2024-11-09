// Copyright 2015 The LUCI Authors.
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

package swarming

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	annopb "go.chromium.org/luci/luciexe/legacy/annotee/proto"

	"go.chromium.org/luci/milo/internal/buildsource/rawpresentation"
)

func toLogDogStream(stream streamclient.FakeStreamData) (*rawpresentation.Stream, error) {
	flags := stream.GetFlags()

	result := &rawpresentation.Stream{
		Closed:     stream.IsClosed(),
		IsDatagram: flags.Type == streamproto.StreamType(logpb.StreamType_DATAGRAM),
		Path:       string(flags.Name),
	}

	if result.IsDatagram {
		result.Data = &annopb.Step{}
		// Assume this is a annopb.Step.
		if err := proto.Unmarshal([]byte(stream.GetStreamData()), result.Data); err != nil {
			return nil, err
		}
	} else {
		result.Text = stream.GetStreamData()
	}

	return result, nil
}

type annotationParser struct {
	stream map[string]streamclient.FakeStreamData
}

func parseAnnotations(scFake streamclient.Fake) (*rawpresentation.Streams, error) {
	fakeData := scFake.Data()

	var parser annotationParser
	parser.stream = make(map[string]streamclient.FakeStreamData, len(fakeData))
	for k, v := range fakeData {
		parser.stream[string(k)] = v
	}
	return parser.ToLogDogStreams()
}

func (c annotationParser) addLogDogTextStream(s *rawpresentation.Streams, ls *annopb.LogdogStream) error {
	var keys []string
	for k := range c.stream {
		keys = append(keys, k)
	}
	ms, ok := c.stream[ls.Name]
	if !ok {
		return fmt.Errorf("could not find text stream %q\n%s", ls.Name, keys)
	}
	lds, err := toLogDogStream(ms)
	if err != nil {
		return fmt.Errorf("could not convert text stream %s\n%s\n%s", ls.Name, err, keys)
	}
	if lds.IsDatagram {
		return fmt.Errorf("expected %s to be a text stream, got a data stream", ls.Name)
	}
	s.Streams[ls.Name] = lds
	return nil
}

// addToStreams adds the set of stream with a given base path to the logdog
// stream map.  A base path is assumed to have a stream named "annotations".
func (c annotationParser) addToStreams(s *rawpresentation.Streams, anno *annopb.Step) error {
	if lds := anno.StdoutStream; lds != nil {
		if err := c.addLogDogTextStream(s, lds); err != nil {
			return fmt.Errorf(
				"encountered error while processing step streams for STDOUT: %s", err)
		}
	}
	if lds := anno.StderrStream; lds != nil {
		if err := c.addLogDogTextStream(s, lds); err != nil {
			return fmt.Errorf(
				"encountered error while processing step streams for STDERR: %s", err)
		}
	}

	// Look for substream.
	for _, link := range anno.GetOtherLinks() {
		lds := link.GetLogdogStream()
		// Not a logdog stream.
		if lds == nil {
			continue
		}

		if err := c.addLogDogTextStream(s, lds); err != nil {
			return fmt.Errorf(
				"encountered error while processing step streams for '%s'\n%s", lds.Name, err)
		}
	}

	// Now do substeps.
	for _, subStepEntry := range anno.Substep {
		substep := subStepEntry.GetStep()
		if substep == nil {
			continue
		}

		if err := c.addToStreams(s, substep); err != nil {
			return err
		}
	}

	return nil
}

func (c annotationParser) ToLogDogStreams() (*rawpresentation.Streams, error) {
	result := &rawpresentation.Streams{}
	result.Streams = map[string]*rawpresentation.Stream{}

	// Register annotation stream.
	const annotationStreamName = "annotations"
	ms, ok := c.stream[annotationStreamName]
	if !ok {
		return nil, fmt.Errorf("could not find stream %s", annotationStreamName)
	}
	ls, err := toLogDogStream(ms)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal stream %s\n%s", annotationStreamName, err)
	}
	if !ls.IsDatagram {
		return nil, fmt.Errorf(
			"annotation stream %s is not a datagram\nText: %s",
			annotationStreamName, ls.Text)
	}
	result.Streams[annotationStreamName] = ls

	// Register any referenced LogDog streams.
	if err := c.addToStreams(result, ls.Data); err != nil {
		return nil, err
	}
	result.MainStream = ls

	if len(c.stream) != len(result.Streams) {
		var mk, lk []string
		for k := range c.stream {
			mk = append(mk, k)
		}
		for k := range result.Streams {
			lk = append(lk, k)
		}
		return nil, fmt.Errorf(
			"number of streams do not match %d vs %d\nMemory:%s\nResult:%s",
			len(c.stream), len(result.Streams), mk, lk)
	}

	return result, nil
}
