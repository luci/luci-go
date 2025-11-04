// Copyright 2021 The LUCI Authors.
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

// Package mailer can be used to send mails through a mailer service.
//
// Install it as a server module and then use Send(...) API.
//
// Using "gae" backend requires running on GAE and having
// "app_engine_apis: true" in the module YAML.
// See https://cloud.google.com/appengine/docs/standard/go/services/access.
package mailer

import (
	"context"
	"encoding/hex"
	"fmt"

	"go.chromium.org/luci/common/data/rand/mathrand"
)

var mailerCtxKey = "go.chromium.org/luci/server/mailer.Mailer"

// Mail represents an email message.
//
// Addresses may be of any form permitted by RFC 822. At least one of To, Cc,
// or Bcc must be non-empty. At least one of TextBody or HTMLBody must be
// non-empty.
type Mail struct {
	// Sender is put into "From" email header field.
	//
	// If empty, some default value will be used. See ModuleOptions.
	Sender string

	// ReplyTo is put into "Reply-To" email header field.
	//
	// Optional.
	ReplyTo string

	// To is put into "To" email header field.
	To []string

	// Cc is put into "Cc" email header field.
	Cc []string

	// Bcc is put into "Bcc" email header field.
	Bcc []string

	// Subject is put into "Subject" email header field.
	Subject string

	// TextBody is a plaintext body of the email message.
	//
	// Can be used together with HTMLBody to send a multipart email.
	TextBody string

	// HTMLBody is an HTML body of the email message.
	//
	// Can be used together with TextBody to send a multipart email.
	HTMLBody string

	// InReplyTo is the ID of the message which this message replies to.
	InReplyTo string

	// References contains the IDs of all messages in the thread.
	References []string
}

// Header represents a header to send with an email message.
type Header struct {
	Key   string
	Value string
}

// Mailer lives in the context and knows how to send Messages.
//
// Must tag transient errors with transient.Tag. All other errors are assumed
// to be fatal.
type Mailer func(ctx context.Context, msg *Mail) (string, error)

// Use replaces the mailer in the context.
//
// Useful in tests to mock the real mailer.
func Use(ctx context.Context, m Mailer) context.Context {
	return context.WithValue(ctx, &mailerCtxKey, m)
}

// Send sends the given message through a mailer client in the context.
//
// Panics if the context doesn't have a mailer. Transient errors are tagged with
// transient.Tag. All other errors are fatal and should not be retried.
func Send(ctx context.Context, msg *Mail) (string, error) {
	m, _ := ctx.Value(&mailerCtxKey).(Mailer)
	if m == nil {
		panic("no mailer in the context")
	}
	return m(ctx, msg)
}

// GenerateMessageID produces a new unique message identifier.
func GenerateMessageID(ctx context.Context) string {
	var blob [20]byte
	if _, err := mathrand.Read(ctx, blob[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("<%s@luci.api.cr.dev>", hex.EncodeToString(blob[:]))
}
