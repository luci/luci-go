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

package templateproto

import (
	"fmt"

	"github.com/luci/luci-go/common/proto"
)

// LoadFile loads a File from a string containing the template text protobuf.
//
// Expects config.Interface to be in the context already.
func LoadFile(data string) (file *File, err error) {
	file = &File{}
	if err = proto.UnmarshalTextML(data, file); err != nil {
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
