// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"fmt"
	"go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/milo/api/config"
	"net/url"
	"strings"
	"text/template"
)

type BugTemplateDetails struct {
	Build *buildbucketpb.Build
}

func fillTemplate(t string, details *BugTemplateDetails, name string) (string, error) {
	tmpl, err := template.New("bug " + name).Option("missingkey=error").Parse(t)
	if err != nil {
		return "", err
	}

	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, details)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}

func summary(bt *config.BugTemplate, details *BugTemplateDetails) (string, error) {
	return fillTemplate(bt.Summary, details, "summary")
}

func description(bt *config.BugTemplate, details *BugTemplateDetails) (string, error) {
	return fillTemplate(bt.Description, details, "description")
}

// makeFeedbackLink attempts to create the feedback link for the build page. If the
// project is not configured for a custom feedback link or an interpolation placeholder
// cannot be satisfied an empty string is returned.
func MakeFeedbackLink(bt *config.BugTemplate, details *BugTemplateDetails) (string, error) {
	summary, err := summary(bt, details)
	if err != nil {
		return "", err
	}
	description, err := description(bt, details)
	if err != nil {
		return "", err
	}

	query := url.Values{
		"summary":     {summary},
		"description": {description},
	}

	if len(bt.Components) > 0 {
		query.Add("components", strings.Join(bt.Components, ","))
	}

	link := url.URL{
		Scheme:   "https",
		Host:     "bugs.chromium.org",
		Path:     fmt.Sprintf("/p/%s/issues/entry", bt.MonorailProject),
		RawQuery: query.Encode(),
	}

	return link.String(), nil
}
