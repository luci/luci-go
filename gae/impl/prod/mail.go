// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package prod

import (
	gae_mail "github.com/luci/gae/service/mail"
	"golang.org/x/net/context"
	"google.golang.org/appengine/mail"
)

// useMail adds a mail service implementation to context, accessible
// by "github.com/luci/gae/service/mail".Raw(c) or the exported mail service
// methods.
func useMail(c context.Context) context.Context {
	return gae_mail.SetFactory(c, func(ci context.Context) gae_mail.RawInterface {
		return mailImpl{AEContext(ci)}
	})
}

type mailImpl struct {
	aeCtx context.Context
}

func (m mailImpl) Send(msg *gae_mail.Message) error {
	return mail.Send(m.aeCtx, msg.ToSDKMessage())
}

func (m mailImpl) SendToAdmins(msg *gae_mail.Message) error {
	return mail.Send(m.aeCtx, msg.ToSDKMessage())
}

func (m mailImpl) GetTestable() gae_mail.Testable { return nil }
