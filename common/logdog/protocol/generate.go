// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc --go_out=Mgoogle/protobuf/timestamp.proto=github.com/luci/luci-go/common/proto/google,Mgoogle/protobuf/duration.proto=github.com/luci/luci-go/common/proto/google:. log.proto butler.proto

// Package protocol contains LogDog protobuf source and generated protobuf data.
//
// The package name here must match the protobuf package name, as the generated
// files will reside in the same directory.
package protocol

import (
	"github.com/golang/protobuf/proto"
)

var _ = proto.Marshal
