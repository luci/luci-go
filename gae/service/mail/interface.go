// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
