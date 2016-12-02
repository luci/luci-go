// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"

	"github.com/maruel/subcommands"
)

var cmdEditDistance = &subcommands.Command{
	UsageLine: `edit-distance [options]`,
	ShortDesc: `Computes the edit distance of two strings.`,
	LongDesc: `Computes the edit distance of two strings using insertion, deletion,
	substitution, and optionally also transposition. This expects the properties
	of the quest to be a single JSON dictionary with two keys "a" and "b". Each
	key should correspond to a string. The result of the job will be a dictionary
	with a key "dist" that is the minimum number of edit operations needed to
	transform a into b.`,
	CommandRun: func() subcommands.CommandRun {
		r := &editDistanceRun{}
		r.registerOptions()
		return r
	},
}

type editDistanceRun struct {
	cmdRun

	useTransposition bool
}

func (e *editDistanceRun) registerOptions() {
	e.cmdRun.registerOptions()
	e.Flags.BoolVar(&e.useTransposition, "use-transposition", false,
		"Include transposition as one of the possible candidates.")
}

// EditParams is the 'parameters' for the edit-distance algorithm.
type EditParams struct {
	A string `json:"a"`
	B string `json:"b"`
}

// EditResult is the normal result data for the edit-distance algorithm.
type EditResult struct {
	Distance uint32 `json:"dist"`

	// OpHistory is a string which says which edit commands were taken. The
	// symbols are:
	//   = - no edit
	//   ~ - substitute
	//   - - delete
	//   + - insert
	//   X - transpose
	//   ! - error
	OpHistory string `json:"op_history"`
	Error     string `json:"error,omitempty"`
}

func (e *editDistanceRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	e.start(a, e, env, cmdEditDistance)

	p := &EditParams{}
	err := json.NewDecoder(bytes.NewBufferString(e.questDesc.Parameters)).Decode(p)
	if err != nil {
		return e.finish(EditResult{Error: err.Error()})
	}

	rslt, stop := e.compute(p)
	if stop {
		return 0
	}

	return e.finish(rslt)
}

var editSymbolLookup = map[string]string{
	"substitution":  "~",
	"insertion":     "+",
	"deletion":      "-",
	"equality":      "=",
	"transposition": "X",
}

func (e *editDistanceRun) compute(p *EditParams) (rslt EditResult, stop bool) {
	// If one string is empty, the edit distance is the length of the other
	// string.
	if len(p.A) == 0 {
		rslt.Distance = uint32(len(p.B))
		rslt.OpHistory = strings.Repeat("+", len(p.B))
		return
	} else if len(p.B) == 0 {
		rslt.Distance = uint32(len(p.A))
		rslt.OpHistory = strings.Repeat("-", len(p.A))
		return
	}

	// If both strings are exactly equality, the distance between them is 0.
	if p.A == p.B {
		rslt.OpHistory = strings.Repeat("=", len(p.A))
		return
	}

	toDep := []interface{}{
		&EditParams{ // substitution || equality
			A: p.A[1:],
			B: p.B[1:],
		},
		&EditParams{ // deletion (remove a char from A)
			A: p.A[1:],
			B: p.B,
		},
		&EditParams{ // insertion (add the char to A)
			A: p.A,
			B: p.B[1:],
		},
	}
	if e.useTransposition && len(p.A) > 1 && len(p.B) > 1 {
		if p.A[0] == p.B[1] && p.A[1] == p.B[0] {
			toDep = append(toDep, &EditParams{ // transposition
				A: p.A[2:],
				B: p.B[2:],
			})
		}
	}

	opNames := []string{
		"substitution", "deletion", "insertion", "transposition",
	}
	if p.A[0] == p.B[0] {
		opNames[0] = "equality"
	}

	retval := uint32(math.MaxUint32)
	opname := ""
	opchain := ""

	depsData, stop := e.depOn(toDep...)
	if stop {
		return
	}

	for i, dep := range depsData {
		result := &EditResult{}
		err := json.NewDecoder(bytes.NewBufferString(dep)).Decode(result)
		if err != nil {
			rslt.Error = err.Error()
			return
		}

		opName := opNames[i]
		if result.Error != "" {
			rslt.OpHistory = "!" + result.OpHistory
			rslt.Error = result.Error
			return
		}
		cost := result.Distance
		if opName != "equality" {
			// all operations (except equality) cost 1
			cost++
		}
		if cost < retval {
			retval = cost
			opname = opName
			opchain = result.OpHistory
		}
	}

	rslt.Distance = retval
	rslt.OpHistory = editSymbolLookup[opname] + opchain
	return
}
