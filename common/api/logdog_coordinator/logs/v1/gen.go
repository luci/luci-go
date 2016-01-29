// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate cproto

// Package logs contains Verison 1 of the LogDog Coordinator user interface.
//
// The package name here must match the protobuf package name, as the generated
// files will reside in the same directory.
package logs

import (
	"github.com/golang/protobuf/proto"
)

var _ = proto.Marshal
