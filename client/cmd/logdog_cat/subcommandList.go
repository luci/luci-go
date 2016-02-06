// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/luci/luci-go/common/logdog/coordinator"
	log "github.com/luci/luci-go/common/logging"
	"github.com/maruel/subcommands"
)

const (
	// defaultListResults is the default number of list results to return.
	defaultListResults = 200
)

type listCommandRun struct {
	subcommands.CommandRunBase

	recursive bool
	long      bool
	purged    bool
}

func newListCommand() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ls",
		ShortDesc: "List log stream hierarchy space.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &listCommandRun{}

			fs := cmd.GetFlags()
			fs.BoolVar(&cmd.recursive, "r", false, "List recursively.")
			fs.BoolVar(&cmd.long, "l", false, "Perform long listing, showing stream details.")
			fs.BoolVar(&cmd.purged, "purged", false, "Include purged streams in listing (admin-only).")
			return cmd
		},
	}
}

func (cmd *listCommandRun) Run(scApp subcommands.Application, args []string) int {
	a := scApp.(*application)

	if len(args) == 0 {
		args = []string{""}
	}

	bio := bufio.NewWriter(os.Stdout)
	defer bio.Flush()

	for _, path := range args {
		o := coordinator.ListOptions{
			Recursive:   cmd.recursive,
			StreamsOnly: cmd.recursive,
			State:       cmd.long,
			Purged:      cmd.purged,
		}

		err := a.coord.List(a, path, o, func(lr *coordinator.ListResult) bool {
			p := lr.Path
			if cmd.recursive {
				p = string(lr.FullPath())
			}

			if lr.State == nil {
				fmt.Fprintf(bio, "%s\n", p)
			} else {
				fmt.Fprintf(bio, "%s\t[%s]\n", p, cmd.attributes(lr))
			}
			return true
		})
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       path,
			}.Errorf(a, "Failed to list path.")
			return 1
		}
	}
	return 0
}

func (*listCommandRun) attributes(lr *coordinator.ListResult) string {
	var parts []string

	if s := lr.State; s != nil {
		if d := s.Desc; d != nil {
			p := []string{
				d.GetTimestamp().Time().String(),
				fmt.Sprintf("type:%s", d.StreamType.String()),
			}

			if t := d.GetTags(); len(t) > 0 {
				tstr := make([]string, 0, len(t))
				for k, v := range t {
					if v == "" {
						tstr = append(tstr, k)
					} else {
						tstr = append(tstr, fmt.Sprintf("%s=%s", k, v))
					}
				}
				sort.Strings(tstr)
				p = append(p, fmt.Sprintf("tags:{%s}", strings.Join(tstr, ", ")))
			}

			parts = append(parts, p...)
		}

		if st := s.State; st != nil {
			if st.TerminalIndex >= 0 {
				parts = append(parts, fmt.Sprintf("terminal:%d", st.TerminalIndex))
			} else {
				parts = append(parts, "streaming")
			}

			if st.Archived {
				parts = append(parts, "archived")
			}

			if st.Purged {
				parts = append(parts, "purged")
			}
		}
	}

	return strings.Join(parts, " ")
}
