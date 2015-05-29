// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package replication_test

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"

	"github.com/luci/luci-go/server/common/auth/replication"
)

func encodeAndTrimEqual(b []byte) string {
	str := base64.StdEncoding.EncodeToString(b)
	return strings.TrimRight(str, "=")
}

func TestDecodeLinkTicketBasic(t *testing.T) {
	ticket := &replication.ServiceLinkTicket{
		PrimaryId:   proto.String("dummyid"),
		PrimaryUrl:  proto.String("http://dummy"),
		GeneratedBy: proto.String("dummy"),
		Ticket:      []byte("dummyticket"),
	}
	m, err := proto.Marshal(ticket)
	if err != nil {
		t.Fatalf("proto.Marshal()=%q,_; want <nil>", err)
	}
	str := encodeAndTrimEqual(m)
	dec, err := replication.DecodeLinkTicket(str)
	if err != nil {
		t.Errorf("DecodeLinkTicket(%q) = _, %q; want <nil>", str, err)
	}
	if !reflect.DeepEqual(ticket, dec) {
		t.Errorf("DecodeLinkTicket(%q) = %q; want %q", str, proto.MarshalTextString(dec), proto.MarshalTextString(ticket))
	}
}

func TestDecodeLinkTicketShouldHandleWebSafeBase64(t *testing.T) {
	ticket := &replication.ServiceLinkTicket{
		PrimaryId:   proto.String("dummyid"),
		PrimaryUrl:  proto.String("http://dummy"),
		GeneratedBy: proto.String("dummy"),
		Ticket:      []byte("dummyticket"),
	}
	m, err := proto.Marshal(ticket)
	if err != nil {
		t.Fatalf("proto.Marshal()=%q,_; want <nil>", err)
	}
	testValue := base64.StdEncoding.EncodeToString(m)
	if !strings.HasSuffix(testValue, "=") {
		t.Fatalf("ticket is not expected pattern %q. this test is meaningless without padding", testValue)
	}
	str := encodeAndTrimEqual(m)
	dec, err := replication.DecodeLinkTicket(str)
	if err != nil {
		t.Errorf("DecodeLinkTicket(%q) = _, %q; want <nil>", str, err)
	}
	if !reflect.DeepEqual(ticket, dec) {
		t.Errorf("DecodeLinkTicket(%q) = %q; want %q", str, proto.MarshalTextString(dec), proto.MarshalTextString(ticket))
	}
}

func TestDecodeLinkTicketShouldErrorBrokenProto(t *testing.T) {
	str := "broken"
	_, err := replication.DecodeLinkTicket(str)
	if err == nil {
		t.Errorf("DecodeLinkTicket(%q) = _, <nil>; want error", str)
	}
}
