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

package cron

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"net/url"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/portal"
)

type portalPage struct {
	portal.BasePage
}

func (portalPage) Title(ctx context.Context) (string, error) {
	return "Cron handlers", nil
}

func (portalPage) Overview(ctx context.Context) (template.HTML, error) {
	text := template.HTML(`
		<p>This page allows to directly invoke a registered cron handler. It is
		primarily intended for use locally during development to manually test
		handlers.
	`)
	if len(Default.handlerIDs()) != 0 {
		text += template.HTML(`Clicking one of the buttons below will result in
			the direct execution of the corresponding cron handler right inside the
			process that handled this button click request.</p>
		`)
	} else {
		text += template.HTML(
			`Note that no cron handlers are registered now, so this page is mostly
			empty. It will have more buttons when cron handlers are added in the
			service code.
			</p>
		`)
	}
	return text, nil
}

func (portalPage) Actions(ctx context.Context) ([]portal.Action, error) {
	var actions []portal.Action
	for _, id := range Default.handlerIDs() {
		actions = append(actions, portal.Action{
			ID:    genActionID(id),
			Title: fmt.Sprintf("Run %q now", id),
			Callback: func(ctx context.Context) (string, template.HTML, error) {
				start := clock.Now(ctx)
				err := Default.executeHandlerByID(ctx, id)
				dur := clock.Since(ctx, start)
				if err != nil {
					return "", "", errors.Annotate(err, "execution of cron handler %q failed after %s", id, dur).Err()
				}
				report := fmt.Sprintf(
					`<p>Execution of cron handler "%s" finished successfully after %s.</p>`,
					html.EscapeString(id), dur,
				)
				return "Success", template.HTML(report), nil
			},
		})
	}
	return actions, nil
}

func genActionID(id string) string {
	return fmt.Sprintf("run-%s", url.QueryEscape(id))
}

func init() {
	portal.RegisterPage("cron", portalPage{})
}
