// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package recordio implements a basic RecordIO reader and writer.
//
// Each RecordIO frame begins with a Uvarint (
// http://golang.org/pkg/encoding/binary/#Uvarint) containing the size of the
// frame, followed by that many bytes of frame data.
//
// The frame protocol does not handle data integrity; that is left to the
// outer protocol or medium which uses the frame.
package recordio
