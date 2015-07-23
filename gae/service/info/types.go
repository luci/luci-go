// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package info

// Certificate represents a public certificate for the app.
type Certificate struct {
	KeyName string
	Data    []byte // PEM-encoded X.509 certificate
}
