// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package streamproto

import (
	"encoding/json"
	"flag"
	"fmt"
	"testing"

	"github.com/luci/luci-go/common/proto/logdog/logpb"
	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamType(t *testing.T) {
	Convey(`A StreamType flag`, t, func() {
		value := StreamType(0)

		fs := flag.NewFlagSet("Testing", flag.ContinueOnError)
		fs.Var(&value, "stream-type", "StreamType test.")

		Convey(`Can be loaded as a flag.`, func() {
			err := fs.Parse([]string{"-stream-type", "datagram"})
			So(err, ShouldBeNil)
			So(value, ShouldEqual, logpb.LogStreamDescriptor_DATAGRAM)
		})

		Convey(`Will unmmarshal from JSON.`, func() {
			var s struct {
				Value StreamType `json:"value"`
			}

			err := json.Unmarshal([]byte(`{"value": "text"}`), &s)
			So(err, ShouldBeNil)
			So(s.Value, ShouldEqual, logpb.LogStreamDescriptor_TEXT)
		})

		Convey(`Will marshal to JSON.`, func() {
			var s struct {
				Value StreamType `json:"value"`
			}
			s.Value = StreamType(logpb.LogStreamDescriptor_BINARY)

			v, err := json.Marshal(&s)
			So(err, ShouldBeNil)
			So(string(v), ShouldResemble, `{"value":"binary"}`)
		})

		for _, t := range []logpb.LogStreamDescriptor_StreamType{
			logpb.LogStreamDescriptor_TEXT,
			logpb.LogStreamDescriptor_BINARY,
			logpb.LogStreamDescriptor_DATAGRAM,
		} {
			Convey(fmt.Sprintf(`Stream type [%s] has a default content type.`, t), func() {
				st := StreamType(t)
				So(st.DefaultContentType(), ShouldNotEqual, "")
			})
		}
	})
}
