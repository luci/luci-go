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

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"go.chromium.org/luci/client/flagpb"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	dm "go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/grpc/prpc"
)

var cmdVisQuery = &subcommands.Command{
	UsageLine: `vis [options] [args for rpc command]`,
	ShortDesc: "Runs a DM WalkGraph query and visualizes the result to a .dot file.",
	LongDesc: `This command runs a WalkGraph query repeatedly, overwriting the .dot
	file specified with a visual representation of the graph.`,
	CommandRun: func() subcommands.CommandRun {
		r := &visQueryRun{}
		r.registerOptions()
		return r
	},
}

type visQueryRun struct {
	cmdRun
	host       string
	path       string
	sequence   bool
	includeAll bool
}

func (r *visQueryRun) registerOptions() {
	r.Flags.StringVar(&r.host, "host", ":8080",
		"The host to connect to")
	r.Flags.StringVar(&r.path, "path", "",
		"The output path for the .dot file. Leave empty to query once and print the result stdout.")
	r.Flags.BoolVar(&r.sequence, "sequence", false,
		"Use `path` as the base path for a sequence of pngs. `path` must not be empty.")
	r.Flags.BoolVar(&r.includeAll, "include-all", false,
		"Modify query to have all 'Include' options.")
}

var alphabet = []rune{
	// lower greek
	'α', 'β', 'γ', 'δ', 'ε', 'ζ', 'η', 'θ', 'ι', 'κ', 'λ', 'μ', 'ν', 'ξ', 'ο',
	'π', 'ρ', 'ς', 'τ', 'υ', 'φ', 'χ', 'ψ', 'ω',

	// lower english
	//'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
	//'q', 'r', 's', 't', 'w', 'y', 'z',

	// upper greek
	'Α', 'Β', 'Γ', 'Δ', 'Ε', 'Ζ', 'Η', 'Θ', 'Ι', 'Κ', 'Λ', 'Μ', 'Ν', 'Ξ', 'Ο',
	'Π', 'Ρ', 'Σ', 'Τ', 'Υ', 'Φ', 'Χ', 'Ψ', 'Ω',

	// upper english
	//'C', 'D', 'F', 'G', 'J', 'L', 'Q', 'R', 'S', 'U', 'V', 'W',
}

type name uint

var alphabetLen = name(len(alphabet))

func (n name) runes() []rune {
	if n < alphabetLen {
		return alphabet[n : n+1]
	}
	return append((n/alphabetLen - 1).runes(), alphabet[n%alphabetLen])
}

func (n name) String() string {
	return string(n.runes())
}

var alphabetLock = sync.Mutex{}
var curAlphabetName = name(0)
var alphabetMap = map[string]name{}

func getColor(a *dm.Attempt) string {
	switch a.Data.State() {
	case dm.Attempt_SCHEDULING:
		return "cadetblue1"
	case dm.Attempt_EXECUTING:
		return "darkorchid"
	case dm.Attempt_WAITING:
		return "gold"
	case dm.Attempt_FINISHED:
		return "chartreuse"
	case dm.Attempt_ABNORMAL_FINISHED:
		return "crimson"
	}
	return "deeppink"
}

func nameFor(qid string) string {
	alphabetLock.Lock()
	defer alphabetLock.Lock()
	if curName, ok := alphabetMap[qid]; ok {
		return curName.String()
	}
	ret := curAlphabetName
	alphabetMap[qid] = ret
	curAlphabetName++
	return ret.String()
}

func renderDotFile(gdata *dm.GraphData) string {
	buf := &bytes.Buffer{}

	indent := 0
	idt := func() string { return strings.Repeat(" ", indent) }
	p := func(format string, args ...interface{}) {
		indent -= strings.Count(format, "}")
		fmt.Fprintf(buf, idt()+format+"\n", args...)
		indent += strings.Count(format, "{")
	}

	edges := []string{}

	p("digraph {")

	sortedQids := make([]string, 0, len(gdata.Quests))
	for qid := range gdata.Quests {
		sortedQids = append(sortedQids, qid)
	}
	sort.Strings(sortedQids)

	for _, qid := range sortedQids {
		q := gdata.Quests[qid]
		qName := nameFor(qid)
		p("subgraph cluster_%s {", qName)
		p("label=%q", qName)
		for aid, a := range q.Attempts {
			if a.DNE {
				continue
			}
			color := getColor(a)
			p(`"%s:%d" [style=filled label="%d" fillcolor=%s]`, qName, aid, aid, color)
			if a.FwdDeps != nil {
				for depQid, aNums := range a.FwdDeps.To {
					depQName := nameFor(depQid)
					for _, num := range aNums.Nums {
						edges = append(edges, fmt.Sprintf(`"%s:%d" -> "%s:%d"`, qName, aid, depQName, num))
					}
				}
			}
		}
		p("}")
	}

	sort.Strings(edges)
	for _, e := range edges {
		p(e)
	}
	p("}")

	return buf.String()
}

func runQuery(c context.Context, dc dm.DepsClient, query *dm.WalkGraphReq) (ret *dm.GraphData, err error) {
	query = proto.Clone(query).(*dm.WalkGraphReq)
	ret = &dm.GraphData{}
	for c.Err() == nil {
		newRet := (*dm.GraphData)(nil)
		newRet, err = dc.WalkGraph(c, query)
		if err != nil || newRet.HadErrors {
			return
		}
		ret.UpdateWith(newRet)
		if !newRet.HadMore {
			return
		}
		query.Query = ret.ToQuery()
	}
	return
}

func (r *visQueryRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	r.cmd = cmdVisQuery

	c, cancel := context.WithCancel(cli.GetContext(a, r, env))

	if r.path == "" && r.sequence {
		return r.argErr("path is required for sequence")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		signal.Stop(sigChan)
		cancel()
	}()

	query := &dm.WalkGraphReq{}
	if err := flagpb.UnmarshalMessage(args, flagpb.NewResolver(dm.FileDescriptorSet()), query); err != nil {
		logging.WithError(err).Errorf(c, "could not construct query")
		return 1
	}
	if r.includeAll {
		query.Include = dm.MakeWalkGraphIncludeAll()
	}

	client := &prpc.Client{
		Host:    r.host,
		Options: prpc.DefaultOptions(),
	}
	client.Options.Insecure = lhttp.IsLocalHost(r.host)
	dc := dm.NewDepsPRPCClient(client)

	prev := ""
	seq := 0

	for c.Err() == nil {
		gdata, err := runQuery(c, dc, query)
		if err != nil {
			if errors.Any(err, func(err error) bool { return err == context.Canceled }) {
				return 0
			}
			logging.WithError(err).Errorf(c, "error running query")
			return 1
		}

		if gdata != nil {
			newVal := renderDotFile(gdata)
			if prev != newVal {
				prev = newVal

				if r.sequence {
					outfile := fmt.Sprintf("%s%d.png", r.path, seq)
					seq++
					cmd := exec.CommandContext(c, "dot",
						"-Gdpi=300", "-Glwidth=6", "-Glheight=17.6",
						"-Tpng", "-o"+outfile)
					cmd.Stdin = strings.NewReader(newVal)
					err := cmd.Run()
					if err != nil {
						logging.WithError(err).Errorf(c, "error running dot")
						return 1
					}
				} else {
					ofile := os.Stdout
					if r.path != "" {
						ofile, err = os.Create(r.path)
						if err != nil {
							logging.Fields{
								logging.ErrorKey: err,
								"outfile":        r.path,
							}.Errorf(c, "error opening output file")
							return 1
						}
					}
					ofile.WriteString(newVal)
					ofile.Close()
					if r.path == "" {
						return 0
					}
				}
			}
		}

		clock.Sleep(c, time.Second/4)
	}
	return 0
}
