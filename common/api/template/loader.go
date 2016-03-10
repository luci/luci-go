// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package template

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/proto"
)

// LoadFile loads a File by configSet and path.
func LoadFile(c context.Context, configSet, path string) (file *File, vers string, err error) {
	templateData, err := config.Get(c).GetConfig(configSet, path, false)
	if err != nil {
		return
	}
	file = &File{}
	vers = templateData.ContentHash
	if err = proto.UnmarshalTextML(templateData.Content, file); err != nil {
		return
	}
	err = file.Normalize()
	return
}

// Render renders the specified template with the given parameters.
func (f *File) Render(spec *Specifier) (string, error) {
	ret, err := f.Template[spec.TemplateName].Render(spec.Params)
	if err != nil {
		err = fmt.Errorf("rendering %q: %s", spec.TemplateName, err)
	}
	return ret, err
}

// RenderL renders a specified template with go literal arguments.
func (f *File) RenderL(templName string, params LiteralMap) (ret string, err error) {
	spec := &Specifier{TemplateName: templName}
	spec.Params, err = params.Convert()
	if err != nil {
		return
	}
	return f.Render(spec)
}
