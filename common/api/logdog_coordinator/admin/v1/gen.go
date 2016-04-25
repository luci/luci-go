// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate cproto
//go:generate svcdec -type AdminServer

// Package logdog contains Version 1 of the LogDog Coordinator service
// interface.
//
// The package name here must match the protobuf package name, as the generated
// files will reside in the same directory.
package logdog

import (
	"github.com/golang/protobuf/proto"
)

var _ = proto.Marshal
