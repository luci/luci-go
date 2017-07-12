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

package lucictx

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/luci/luci-go/common/system/environ"
)

// Exported represents an exported on-disk LUCI_CONTEXT file. It lives for
// exactly the life of the callback function in Export.
type Exported interface {
	io.Closer

	// SetInCmd sets/replaces the LUCI_CONTEXT environment variable in an
	// exec.Cmd.
	SetInCmd(c *exec.Cmd)

	// SetInEnviron sets/replaces the LUCI_CONTEXT in an environ.Env object.
	SetInEnviron(env environ.Env)
}

type baseExport struct {
	closed bool
}

func (e baseExport) assertOpen() {
	if e.closed {
		panic("Using closed lucictx.Exported object")
	}
}

func (e *baseExport) Close() error {
	e.assertOpen()
	e.closed = true
	return nil
}

type liveExport struct {
	baseExport
	path string

	previousEnvValue *string
}

func (e *liveExport) SetInCmd(c *exec.Cmd) {
	e.assertOpen()
	pfx := EnvKey + "="
	newVal := pfx + e.path
	if c.Env == nil {
		c.Env = os.Environ()
	}
	for i, l := range c.Env {
		if strings.HasPrefix(strings.ToUpper(l), pfx) {
			c.Env[i] = newVal
			return
		}
	}
	c.Env = append(c.Env, newVal)
}

func (e *liveExport) SetInEnviron(env environ.Env) {
	e.assertOpen()
	env.Set(EnvKey, e.path)
}

func (e *liveExport) Close() error {
	e.baseExport.Close()
	if err := os.Remove(e.path); err != nil {
		fmt.Fprintf(os.Stderr, "Could not remove LUCI_CONTEXT file %q: %s", e.path, err)
	}
	return nil
}

type nullExport struct {
	baseExport
}

func (n *nullExport) SetInCmd(*exec.Cmd)       { n.assertOpen() }
func (n *nullExport) SetInEnviron(environ.Env) { n.assertOpen() }
