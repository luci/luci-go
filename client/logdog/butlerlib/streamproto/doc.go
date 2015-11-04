// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package streamproto describes the protocol primitives used by LogDog/Butler
// for stream negotiation.
//
// A LogDog Butler client wishing to create a new LogDog stream can use the
// Flags type to configure/send the stream.
//
// Internally, LogDog represents the Flags properties with the Properties type.
package streamproto
