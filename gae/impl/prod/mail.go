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

package prod

import (
	gae_mail "go.chromium.org/gae/service/mail"
	"golang.org/x/net/context"
	"google.golang.org/appengine/mail"
)

// useMail adds a mail service implementation to context, accessible
// by "go.chromium.org/gae/service/mail".Raw(c) or the exported mail service
// methods.
func useMail(c context.Context) context.Context {
	return gae_mail.SetFactory(c, func(ci context.Context) gae_mail.RawInterface {
		return mailImpl{getAEContext(ci)}
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
