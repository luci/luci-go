// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mail

import (
	"golang.org/x/net/context"
)

// RawInterface is the interface for all of the mail methods.
//
// These replicate the methods found here:
// https://godoc.org/google.golang.org/appengine/mail
type RawInterface interface {
	Send(msg *Message) error
	SendToAdmins(msg *Message) error

	GetTestable() Testable
}

// Send sends an e-mail message.
func Send(c context.Context, msg *Message) error {
	return Raw(c).Send(msg)
}

// SendToAdmins sends an e-mail message to application administrators.
func SendToAdmins(c context.Context, msg *Message) error {
	return Raw(c).SendToAdmins(msg)
}

// GetTestable returns a testable extension interface, or nil if one is not
// available.
func GetTestable(c context.Context) Testable {
	return Raw(c).GetTestable()
}
