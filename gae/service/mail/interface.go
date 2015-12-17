// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mail

// Interface is the interface for all of the mail methods.
//
// These replicate the methods found here:
// https://godoc.org/google.golang.org/appengine/mail
type Interface interface {
	Send(msg *Message) error
	SendToAdmins(msg *Message) error

	Testable() Testable
}
