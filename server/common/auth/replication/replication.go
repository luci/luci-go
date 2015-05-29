// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package replication

//go:generate protoc --go_out=. replication.proto

import (
	"encoding/base64"
	"strings"

	"github.com/golang/protobuf/proto"
)

// DecodeLinkTicket decodes Base64 encoded service link ticket.
func DecodeLinkTicket(t string) (*ServiceLinkTicket, error) {
	// proper padding is needed to make base64 library work.
	if l := len(t) % 4; l != 0 {
		t += strings.Repeat("=", 4-l)
	}
	dec, err := base64.URLEncoding.DecodeString(t)
	if err != nil {
		return nil, err
	}

	ticket := &ServiceLinkTicket{}
	if err = proto.Unmarshal(dec, ticket); err != nil {
		return nil, err
	}
	return ticket, nil
}
