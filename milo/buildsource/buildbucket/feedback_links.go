// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbucket

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"text/template"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/milo/api/config"
)

func fillTemplate(t string, data interface{}, name string) (string, error) {
	tmpl, err := template.New("bug " + name).Option("missingkey=error").Parse(t)
	if err != nil {
		return "", err
	}

	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, data)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}

func summary(bt *config.BugTemplate, data interface{}) (string, error) {
	return fillTemplate(bt.Summary, data, "summary")
}

func description(bt *config.BugTemplate, data interface{}) (string, error) {
	return fillTemplate(bt.Description, data, "description")
}

// makeBuildBugLink attempts to create the feedback link for the build page. If the
// project is not configured for a custom build bug link or an interpolation placeholder
// cannot be satisfied an empty string is returned.
func MakeBuildBugLink(bt *config.BugTemplate, data interface{}) (string, error) {
	summary, err := summary(bt, data)
	if err != nil {
		return "", errors.Annotate(err, "Unable to make summary for build bug link.").Err()
	}
	description, err := description(bt, data)
	if err != nil {
		return "", errors.Annotate(err, "Unable to make description for build bug link.").Err()
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
