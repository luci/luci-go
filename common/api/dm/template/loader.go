// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dmTemplate

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/api/template"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/proto"
)

// LoadFile loads a File by configSet and path.
func LoadFile(c context.Context, project, ref string) (file *File, vers string, err error) {
	cfgSet := "projects/" + project
	if ref != "" {
		cfgSet += "/" + ref
	}
	templateData, err := config.Get(c).GetConfig(cfgSet, "dm/quest_templates.cfg", false)
	if err != nil {
		return
	}
	file = &File{}
	vers = templateData.ContentHash
	err = proto.UnmarshalTextML(templateData.Content, file)
	if err != nil {
		return
	}
	err = file.Normalize()
	return
}

// Render renders the specified template with the given parameters.
func (f *File) Render(spec *template.Specifier) (*dm.Quest_Desc, error) {
	t := f.Template[spec.TemplateName]
	desc, err := t.Payload.Render(spec.Params)
	if err != nil {
		err = fmt.Errorf("rendering %q: %s", spec.TemplateName, err)
	}
	return &dm.Quest_Desc{
		DistributorConfigName: t.DistributorConfigName,
		JsonPayload:           desc,
	}, nil
}
