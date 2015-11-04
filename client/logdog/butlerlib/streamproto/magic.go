// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package streamproto

var (
	// ProtocolFrameHeaderMagic is the number at the beginning of streams that
	// identifies the stream handshake version.
	ProtocolFrameHeaderMagic = []byte("BTLR1\x1E")
)
