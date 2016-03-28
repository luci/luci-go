// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate cproto -discovery=false -import-path "github.com/luci/luci-go/common/proto/google"

// Package google contains local copies of the standard Google Proto3 protobufs.
package google

import (
	"github.com/golang/protobuf/proto"
)

var _ = proto.Marshal
