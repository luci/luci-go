// Copyright 2019 The LUCI Authors.
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

package internal

import (
	"context"
	"fmt"
	"html/template"
	"runtime"
	"sync"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/portal"
)

const (
	blockProfileRate = 100
	mutexProfileRate = 10
)

var (
	profilingEnabled bool
	profilingMutex   sync.Mutex
)

func setProfilingEnabled(ctx context.Context, enabled bool) {
	profilingMutex.Lock()
	defer profilingMutex.Unlock()
	if profilingEnabled != enabled {
		profilingEnabled = enabled
		if enabled {
			runtime.SetBlockProfileRate(blockProfileRate)
			runtime.SetMutexProfileFraction(mutexProfileRate)
			logging.Warningf(ctx, "Contention profiling is now enabled")
		} else {
			runtime.SetBlockProfileRate(0)
			runtime.SetMutexProfileFraction(0)
			logging.Warningf(ctx, "Contention profiling is now disabled")
		}
	}
}

func isProfilingEnabled() bool {
	profilingMutex.Lock()
	defer profilingMutex.Unlock()
	return profilingEnabled
}

type pprofPage struct {
	portal.BasePage
}

func (pprofPage) Title(ctx context.Context) (string, error) {
	return "Profiling options", nil
}

func (pprofPage) Overview(ctx context.Context) (template.HTML, error) {
	curValue := "disabled"
	if isProfilingEnabled() {
		curValue = "enabled"
	}
	return template.HTML(fmt.Sprintf(`
<p>This page allows to enable the collection of goroutine blocking and mutex
contention events from this process, by calling:</p>
<pre>
  runtime.SetBlockProfileRate(%d)
  runtime.SetMutexProfileFraction(%d)
</pre>

<p>The current value of this setting is <b>%s</b>.</p>

<p>This slows down the process and should be used sparingly only when
debugging performance issues. This setting affects only this single specific
process and it is not preserved between process restarts.</p>

<p>See <a href="https://golang.org/pkg/runtime/#SetBlockProfileRate">SetBlockProfileRate</a> and
<a href="https://golang.org/pkg/runtime/#SetMutexProfileFraction">SetMutexProfileFraction</a>
for more info.</p>

<p>All available profiles are listed at <a href="/debug/pprof/">/debug/pprof</a>.</p>

<p>To visualize some profile (e.g. <code>heap</code>), run e.g.:</p>
<pre>
  go tool pprof -png \
      http://127.0.0.1:8900/debug/pprof/heap > out.png
</pre>

<p>To save a profile as a compressed protobuf message for later analysis:</p>
<pre>
  go tool pprof -proto \
      http://127.0.0.1:8900/debug/pprof/heap > out.pb.gz
</pre>
`,
		blockProfileRate, mutexProfileRate, curValue)), nil
}

func (pprofPage) Actions(ctx context.Context) ([]portal.Action, error) {
	var profilingAction portal.Action
	if !isProfilingEnabled() {
		profilingAction = portal.Action{
			ID:    "EnableProfiling",
			Title: "Enable contention profiling",
			Callback: func(ctx context.Context) (string, template.HTML, error) {
				setProfilingEnabled(ctx, true)
				return "Done", `<p>Contention profiling is now enabled.</p>`, nil
			},
		}
	} else {
		profilingAction = portal.Action{
			ID:    "DisableProfiling",
			Title: "Disable contention profiling",
			Callback: func(ctx context.Context) (string, template.HTML, error) {
				setProfilingEnabled(ctx, false)
				return "Done", `<p>Contention profiling is now disabled.</p>`, nil
			},
		}
	}
	return []portal.Action{profilingAction}, nil
}

func init() {
	portal.RegisterPage("pprof", pprofPage{})
}
