// Copyright 2016 The LUCI Authors.
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

package dmTemplate

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/data/text/templateproto"
	dm "github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"
)

// LoadFile loads a File by configSet and path.
func LoadFile(c context.Context, project, ref string) (file *File, vers string, err error) {
	// If ref is "", this will be a standard project config set.
	cfgSet := cfgtypes.RefConfigSet(cfgtypes.ProjectName(project), ref)

	file = &File{}
	var meta cfgclient.Meta
	if err = cfgclient.Get(c, cfgclient.AsService, cfgSet, "dm/quest_templates.cfg", textproto.Message(file), &meta); err != nil {
		return
	}
	vers = meta.ContentHash
	err = file.Normalize()
	return
}

// Render renders the specified template with the given parameters.
func (f *File) Render(spec *templateproto.Specifier) (*dm.Quest_Desc, error) {
	t := f.Template[spec.TemplateName]
	params, err := t.Parameters.Render(spec.Params)
	if err != nil {
		return nil, fmt.Errorf("rendering %q: field distributor parameters: %s", spec.TemplateName, err)
	}
	distribParams, err := t.DistributorParameters.Render(spec.Params)
	if err != nil {
		return nil, fmt.Errorf("rendering %q: field distributor parameters: %s", spec.TemplateName, err)
	}
	return &dm.Quest_Desc{
		DistributorConfigName: t.DistributorConfigName,
		Parameters:            params,
		DistributorParameters: distribParams,
		Meta: t.Meta,
	}, nil
}
