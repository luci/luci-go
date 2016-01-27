// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"google.golang.org/grpc"
)

type service struct {
	desc    *grpc.ServiceDesc
	methods map[string]*method
	impl    interface{}
}
