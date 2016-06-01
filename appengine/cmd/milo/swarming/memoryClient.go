// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/appengine/cmd/milo/logdog"
	"github.com/luci/luci-go/client/logdog/butlerlib/streamclient"
	"github.com/luci/luci-go/client/logdog/butlerlib/streamproto"
	miloProto "github.com/luci/luci-go/common/proto/milo"
)

// In-memory datastructure to hold a fake butler client.
type memoryStream struct {
	*streamproto.Properties

	buf bytes.Buffer
	dg  []byte
}

func (s *memoryStream) ToLogDogStream() (*logdog.Stream, error) {
	result := &logdog.Stream{}
	result.Prefix = s.Prefix
	result.Path = s.Name
	result.Data = &miloProto.Step{}

	if len(s.dg) > 0 {
		// Assume this is a datagram.
		err := proto.Unmarshal(s.dg, result.Data)
		if err != nil {
			return nil, err
		}
		result.IsDatagram = true
	} else {
		result.Text = s.buf.String()
		result.IsDatagram = false
	}

	return result, nil
}

func (s *memoryStream) Write(b []byte) (int, error) {
	return s.buf.Write(b)
}

func (s *memoryStream) Close() error {
	return nil
}

func (s *memoryStream) WriteDatagram(b []byte) error {
	s.dg = make([]byte, len(b))
	copy(s.dg, b)
	return nil
}

type memoryClient struct {
	stream map[string]*memoryStream
}

func (c *memoryClient) NewStream(f streamproto.Flags) (streamclient.Stream, error) {
	props := f.Properties()
	if _, ok := c.stream[props.Name]; ok {
		return nil, fmt.Errorf("duplicate stream, %q", props.Name)
	}
	s := memoryStream{
		Properties: props,
	}
	if c.stream == nil {
		c.stream = map[string]*memoryStream{}
	}
	c.stream[s.Name] = &s
	return &s, nil
}

func (c *memoryClient) addLogDogTextStream(lds *logdog.Streams, name string) error {
	var keys []string
	for k := range c.stream {
		keys = append(keys, k)
	}
	ms, ok := c.stream[name]
	if !ok {
		return fmt.Errorf("Could not find text stream %q\n%s", name, keys)
	}
	ls, err := ms.ToLogDogStream()
	if err != nil {
		return fmt.Errorf("Could not convert text stream %s\n%s\n%s", name, err, keys)
	}
	if ls.IsDatagram {
		return fmt.Errorf("Expected %s to be a text stream, got a data stream", name)
	}
	lds.Streams[name] = ls
	return nil
}

// addToStreams adds the set of stream with a given base path to the logdog stream
// map.  A base path is assumed to have a stream named "annotations".
func (c *memoryClient) addToStreams(name string, s *logdog.Streams) (*logdog.Stream, error) {
	fullname := "annotations"
	if name != "" {
		fullname = strings.Join([]string{name, "annotations"}, "/")
	}
	ms, ok := c.stream[fullname]
	if !ok {
		return nil, fmt.Errorf("Could not find stream %s", fullname)
	}
	ls, err := ms.ToLogDogStream()
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal stream %s\n%s", fullname, err)
	}
	if !ls.IsDatagram {
		return nil, fmt.Errorf(
			"Annotation stream %s is not a datagram\nText: %s",
			fullname, ls.Text)
	}
	s.Streams[fullname] = ls

	// Look for substream.
	substreams := []string{}
	for _, link := range ls.Data.GetStepComponent().GetOtherLinks() {
		lds := link.GetLogdogStream()
		// Not a logdog stream.
		if lds == nil {
			continue
		}
		substreams = append(substreams, lds.Name)
	}
	if name == "" {
		substreams = append(substreams, "stdout")
	} else {
		substreams = append(substreams, fmt.Sprintf("%s/stdout", name))
	}
	for _, subname := range substreams {
		err = c.addLogDogTextStream(s, subname)
		if err != nil {
			return nil, fmt.Errorf(
				"Encountered error while processing step streams for '%s'\n%s", name, err)
		}
	}

	// Now do substeps.
	for _, subName := range ls.Data.SubstepLogdogNameBase {
		_, err = c.addToStreams(subName, s)
		if err != nil {
			return nil, err
		}
	}

	return ls, nil
}

func (c *memoryClient) ToLogDogStreams() (*logdog.Streams, error) {
	result := &logdog.Streams{}
	result.Streams = map[string]*logdog.Stream{}
	mls, err := c.addToStreams("", result)
	if err != nil {
		return nil, err
	}
	result.MainStream = mls

	if len(c.stream) != len(result.Streams) {
		var mk, lk []string
		for k := range c.stream {
			mk = append(mk, k)
		}
		for k := range result.Streams {
			lk = append(lk, k)
		}
		return nil, fmt.Errorf(
			"Number of streams do not match %d vs %d\nMemory:%s\nResult:%s",
			len(c.stream), len(result.Streams), mk, lk)
	}

	return result, nil
}
