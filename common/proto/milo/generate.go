// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc --go_out=Mgoogle/protobuf/timestamp.proto=github.com/luci/luci-go/common/proto/google:. annotations.proto

// Package milo contains Milo protobuf definitions.
//
// The package name here must match the protobuf package name, as the generated
// files will reside in the same directory.
package milo

import (
	"github.com/golang/protobuf/proto"
)

var _ = proto.Marshal
