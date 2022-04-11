// Copyright 2022 The LUCI Authors.
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

// build_profiler takes CPU usage of each process during the chrome build.
//
// Usage:
//
//  $ build_profiler autoninja -C out/Release chrome
//
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/process"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
)

type processKey struct {
	pid        int32
	createTime int64
}

type processValue struct {
	cmdline     []string
	cpuTimeStat *cpu.TimesStat
}

// updateProcessMap updates cpuTimeStat of map.
func updateProcessMap(ctx context.Context, m map[processKey]*processValue) error {
	processes, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to call ProcessesWithContext").Err()
	}

	// Ignore error when failed to get process attributes here.
	for _, p := range processes {
		ct, err := p.CreateTimeWithContext(ctx)
		if err != nil {
			continue
		}

		times, err := p.TimesWithContext(ctx)
		if err != nil {
			continue
		}

		pkey := processKey{
			pid:        p.Pid,
			createTime: ct,
		}

		v, ok := m[pkey]
		if !ok {
			cmdline, err := p.CmdlineSliceWithContext(ctx)
			if err != nil {
				continue
			}

			if len(cmdline) == 0 {
				continue
			}

			v = &processValue{
				cmdline: cmdline,
			}
			m[pkey] = v
		}

		v.cpuTimeStat = times
	}

	return nil
}

// getProcessKind classifies processes ran by chromium build.
func getProcessKind(cmd []string) string {
	argv0 := filepath.Base(cmd[0])

	if strings.HasPrefix(argv0, "nasm") || strings.HasPrefix(argv0, "clang") || strings.Contains(argv0, "lld") || strings.HasPrefix(argv0, "javac") || strings.HasPrefix(argv0, "gomacc") {
		return cmd[0]
	}

	if strings.HasPrefix(argv0, "python") || strings.HasPrefix(argv0, "node") {
		return fmt.Sprintf("%s %s", cmd[0], cmd[1])
	}

	if strings.HasPrefix(argv0, "java") {
		for _, v := range cmd {
			if strings.HasSuffix(v, ".jar") {
				return fmt.Sprintf("%s %s", cmd[0], v)
			}
		}
	}

	return fmt.Sprintf("%s", cmd)
}

func main() {
	ctx := context.Background()

	beforeBuildProcesses := make(map[processKey]*processValue)
	if err := updateProcessMap(ctx, beforeBuildProcesses); err != nil {
		log.Fatalf("failed: %v", err)
	}

	duringBuildProcesses := make(map[processKey]*processValue)
	for k, v := range beforeBuildProcesses {
		copiedV := *v
		duringBuildProcesses[k] = &copiedV
	}

	args := os.Args[1:]
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatalf("failed: %v", err)
	}

	var eg errgroup.Group
	eg.Go(func() error {
	loop:
		for {
			select {
			case <-ctx.Done():
				if err := ctx.Err(); errors.Contains(err, context.Canceled) {
					break loop
				}
				return ctx.Err()
			case <-time.After(5 * time.Second):
				if err := updateProcessMap(ctx, duringBuildProcesses); err != nil {
					return err
				}
			}

		}

		return nil
	})

	if err := cmd.Wait(); err != nil {
		log.Fatalf("failed to wait process: %v", err)
	}
	cancel()
	if err := eg.Wait(); err != nil {
		log.Fatalf("failed to wait errorgroup: %v", err)
	}

	for k, v := range beforeBuildProcesses {
		dv := duringBuildProcesses[k].cpuTimeStat
		dv.System -= v.cpuTimeStat.System
		dv.User -= v.cpuTimeStat.User
	}

	processValues := make([]*processValue, 0, len(duringBuildProcesses))
	for _, v := range duringBuildProcesses {
		processValues = append(processValues, v)
	}

	sort.Slice(processValues, func(i, j int) bool {
		istat := processValues[i].cpuTimeStat
		jstat := processValues[j].cpuTimeStat
		return istat.System+istat.User > jstat.System+jstat.User
	})

	// Show 10 most cpu consuming process during the build.
	slowestProcesses := processValues
	if len(slowestProcesses) >= 10 {
		slowestProcesses = slowestProcesses[:10]
	}

	for _, v := range slowestProcesses {
		fmt.Printf("cmd %s, cpu time %f\n", v.cmdline, v.cpuTimeStat.System+v.cpuTimeStat.User)
	}

	groupedProcessMap := make(map[string]float64)
	for _, v := range processValues {
		groupedProcessMap[getProcessKind(v.cmdline)] += v.cpuTimeStat.System + v.cpuTimeStat.User
	}

	type group struct {
		name    string
		cpuTime float64
	}

	var groupedProcesses []group
	for k, v := range groupedProcessMap {
		if v >= 1 {
			groupedProcesses = append(groupedProcesses, group{
				name:    k,
				cpuTime: v,
			})
		}
	}

	sort.Slice(groupedProcesses, func(i, j int) bool {
		return groupedProcesses[i].cpuTime > groupedProcesses[j].cpuTime
	})

	for _, p := range groupedProcesses {
		fmt.Printf("group %s, cpu time %f\n", p.name, p.cpuTime)
	}
}
