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

package apigen

import (
	"os"
	"os/exec"
	"time"

	"github.com/luci/luci-go/common/clock"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

const shutdownWait = 20 * time.Second

// killableCommand is an extension of exec.Cmd that can be easily killed.
type killableCommand struct {
	*exec.Cmd
}

func (cmd *killableCommand) kill(c context.Context) {
	c = log.SetField(c, "cmd", cmd.Path)

	// First, try and nicely kill the process.
	log.Debugf(c, "Sending interrupt to process.")
	cmd.Process.Signal(os.Interrupt)

	processFinishedC := make(chan struct{})
	defer close(processFinishedC)
	go func() {
		total := time.Duration(0)
		t := clock.NewTimer(c)
		for total < shutdownWait {
			t.Reset(time.Second)
			select {
			case <-processFinishedC:
				return

			case <-t.GetC():
				// Signal the process again.
				total += time.Second
				log.Debugf(c, "Process not terminated; sending interrupt to process.")
				cmd.Process.Signal(os.Interrupt)
			}
		}

		log.Debugf(c, "Process did not gracefully exit; sending KILL.")
		cmd.Process.Kill()
	}()

	cmd.Wait()
	log.Debugf(c, "Process terminated.")
}
