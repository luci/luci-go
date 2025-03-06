// Copyright 2025 The LUCI Authors.
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

// Package debugstack has functions for capturing a debugging stack (like
// `debug.Stack()` but with a bit more configurability (able to skip frames and
// filter frames from unwanted packages).
//
// Why not use e.g. `gostackparse`? This is a really good question, but there
// are some differences between debugstack and gostackparse:
//  1. debugstack aims to always re-create the original stacktrace text verbatim
//     (assuming the stack is not filtered), even in the presence of new,
//     unknown, stack frame types. gostackparse drops data (like frame
//     pointer offsets), and does not explicitly capture interleaved frame
//     variants (like haivng elided frames in the middle of other stack frames).
//  2. debugstack has stack filtering functionality.
//  3. debugstack is explicitly aimed at parsing the output of debug.Stack(),
//     not of parsing the multi-stack snapshot that gostackparse is aimed at.
//  4. debugstack only does the minimal amount of parsing necessary to return
//     a filterable stack, whereas gostackparse is trying to extract all
//     semantic information, including goroutine IDs, etc.
//
// This package is not as optimized as gostackparse (as of writing it operates
// at about 50MB/s vs gostackparse's 500MB/s on a macbook m3 pro), though this
// package is a bit more memory efficient (~15% fewer alloc's op).
package debugstack

import (
	"fmt"
	"regexp"
	"strings"
)

// Rule is a single rule which indicates which types of frames it applies to,
// and has multiple ways to drop frames of this kind. If multiple drop rules are
// set, they apply in order (i.e. Drop, then DropN, then DropIfInPkg).
type Rule struct {
	// ApplyTo is a bit mask of the FrameKinds which this Rule applies to.
	ApplyTo FrameKind

	// Drop indicates that you want all frame types in ApplyTo to be removed.
	Drop bool

	// DropN indicates how many of these frames to drop, starting from the top.
	//
	// This N applies independently to each FrameKind indicated by ApplyTo (e.g.
	// 7 with ApplyTo=StackFrameKind|CreatedByFrameKind means that each of
	// StackFrameKind and CreatedByFrameKind will drop up to 7 frames).
	DropN int

	// DropIfInPkg is a list of package regexps to match the package name, for
	// frames which have PkgName.
	DropIfInPkg []*regexp.Regexp
}

type compiledRuleItem struct {
	// dropN < 0 means drop all.
	dropN     int
	dropPkgRe *regexp.Regexp
}

type CompiledRule struct {
	applyTo map[FrameKind]*compiledRuleItem
}

func CompileRules(rules ...Rule) CompiledRule {
	ret := CompiledRule{map[FrameKind]*compiledRuleItem{}}
	excludePkgs := map[FrameKind][]string{}

	for _, r := range rules {
		var allPkgs []string
		for _, pkg := range r.DropIfInPkg {
			allPkgs = append(allPkgs, "(?:"+pkg.String()+")")
		}
		for fk := range r.ApplyTo.MaskAffected() {
			cur := ret.applyTo[fk]
			if cur != nil && cur.dropN < 0 {
				continue
			} else if cur == nil {
				cur = &compiledRuleItem{}
				ret.applyTo[fk] = cur
			}
			if r.Drop {
				ret.applyTo[fk].dropN = -1
				ret.applyTo[fk].dropPkgRe = nil
				delete(excludePkgs, fk)
				continue
			}
			if r.DropN > 0 {
				var curN int
				if cur != nil {
					curN = cur.dropN
				}
				ret.applyTo[fk].dropN = max(curN, r.DropN)
				delete(excludePkgs, fk)
			}
			excludePkgs[fk] = append(excludePkgs[fk], allPkgs...)
		}
	}

	for fk, pkgs := range excludePkgs {
		if len(pkgs) > 0 {
			// we use MustCompile here because we already know that every value in
			// `pkgs` is a valid regexp, since they were provided via regexp.Pattern
			// objects.
			ret.applyTo[fk].dropPkgRe = regexp.MustCompile(
				"^(?:" + strings.Join(pkgs, "|") + ")$")
		}
	}

	return ret
}

// clone duplicates this CompiledRule.
func (r CompiledRule) clone() CompiledRule {
	ret := CompiledRule{make(map[FrameKind]*compiledRuleItem, len(r.applyTo))}
	for fk, cr := range r.applyTo {
		crCopy := *cr
		ret.applyTo[fk] = &crCopy
	}
	return ret
}

// matches returns true iff `f` matches the compiled rule.
//
// This is STATEFUL in r.applyTo[f.Kind].dropN, which is why Filter clones the
// CompiledRule before applying it.
func (r CompiledRule) matches(f Frame) bool {
	cri, ok := r.applyTo[f.Kind]
	if !ok {
		return false
	}
	if cri.dropN < 0 {
		return true
	} else if cri.dropN > 0 {
		cri.dropN--
		return true
	}
	if len(f.PkgName) == 0 {
		return false
	}
	return cri.dropPkgRe != nil && cri.dropPkgRe.MatchString(f.PkgName)
}

// Filter returns a new Trace which contains only Frames which match the
// CompiledRule.
//
// If `elide` is true, this will insert a FramesElidedKind frame wherever one or
// more frames were removed.
func (t Trace) Filter(rule CompiledRule, elide bool) Trace {
	ret := make(Trace, 0, len(t))
	rule = rule.clone()

	elided := 0
	emitElided := func() {
		if elided > 0 {
			if elide {
				ret = append(ret, Frame{
					FramesElidedKind,
					"", "",
					[]string{fmt.Sprintf("...%d frames elided...\n", elided)},
				})
			}
			elided = 0
		}
	}

	for _, frame := range t {
		if rule.matches(frame) {
			elided++
		} else {
			emitElided()
			ret = append(ret, frame)
		}
	}
	emitElided()

	return ret
}
