// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"fmt"

	"github.com/luci/luci-go/client/logdog/butlerlib/streamproto"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
)

// Holds common command-line stream configuration parameters.
type streamConfig struct {
	streamproto.Flags
}

func (s *streamConfig) addFlags(fs *flag.FlagSet) {
	// Set defaults.
	if s.ContentType == "" {
		s.ContentType = string(types.ContentTypeText)
	}
	s.Type = streamproto.StreamType(logpb.LogStreamDescriptor_TEXT)
	s.Tee = streamproto.TeeNone

	fs.Var(&s.Name, "name", "The name of the stream")
	fs.StringVar(&s.ContentType, "content-type", s.ContentType,
		"The stream content type.")
	fs.Var(&s.Type, "type",
		fmt.Sprintf("Input stream type. Choices are: %s",
			streamproto.StreamTypeFlagEnum.Choices()))
	fs.Var(&s.Tee, "tee",
		fmt.Sprintf("Tee the stream through the Butler's output. Options are: %s",
			streamproto.TeeTypeFlagEnum.Choices()))
	fs.Var(&s.Tags, "tag", "Add a key=value tag.")
}

// Converts command-line parameters into a stream.Config.
func (s streamConfig) properties() streamproto.Properties {
	if s.ContentType == "" {
		// Choose content type based on format.
		s.ContentType = string(s.Type.DefaultContentType())
	}
	return *s.Properties()
}
