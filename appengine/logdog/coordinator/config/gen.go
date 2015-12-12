// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc --go_out=. config.proto

// Package config contains LogDog Coordinator configuration protobufs.
//
// The package name here must match the protobuf package name, as the generated
// files will reside in the same directory.
package config

import (
	"github.com/golang/protobuf/proto"
)

var _ = proto.Marshal
