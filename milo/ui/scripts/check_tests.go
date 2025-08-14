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

// Command check-tests runs a test suite multiple times to check for
// flakiness and performance.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	runs    = flag.Int("n", 50, "The number of times to run the test.")
	verbose = flag.Bool("v", false, "Show the full output of each test run.")
	perf    = flag.Bool("p", false, "Collect and display performance metrics.")
	help    = flag.Bool("h", false, "Show this help message.")
)

const (
	red       = "\033[0;31m"
	green     = "\033[0;32m"
	yellow    = "\033[1;33m"
	noColor   = "\033[0m"
	clearLine = "\r\033[K" // ANSI escape code to clear the current line.
)

func showHelp() {
	fmt.Println("Usage: check-tests [options] <test_matcher>")
	fmt.Println("")
	fmt.Println("Runs a test suite multiple times to check for flakiness and performance.")
	fmt.Println("")
	fmt.Println("Options:")
	flag.PrintDefaults()
}

func parseArgs(args []string) (string, error) {
	flag.CommandLine.Parse(args)
	if *help {
		showHelp()
		os.Exit(0)
	}

	matcher := flag.Arg(0)
	if matcher == "" {
		return "", fmt.Errorf("no test matcher provided")
	}
	return matcher, nil
}

func main() {
	matcher, err := parseArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("%sError: %s.%s\n", red, err, noColor)
	}
	npmArgs := []string{"test", "--", matcher}

	var failures, passes int
	startTime := time.Now()
	var allTimings []float64
	testTimings := make(map[string][]float64)

	// Get the directory of the ui folder.
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	uiDir := filepath.Join(wd, "milo", "ui")

	for i := 1; i <= *runs; i++ {
		fmt.Printf(clearLine)

		// Progress bar
		progressBar := "["
		for j := 0; j < passes; j++ {
			progressBar += green + "#" + noColor
		}
		for j := 0; j < failures; j++ {
			progressBar += red + "#" + noColor
		}
		for j := 0; j < (*runs - i + 1); j++ {
			progressBar += " "
		}
		progressBar += "]"

		// Time estimation
		var estimate string
		if i > 1 {
			elapsed := time.Since(startTime)
			avgTime := elapsed / time.Duration(i-1)
			remaining := time.Duration(*runs-i+1) * avgTime
			estimate = fmt.Sprintf(" (est. %s remaining)", remaining.Round(time.Second))
		}

		fmt.Printf("Running %d/%d: %s%d passed%s, %s%d failed%s %s%s", i, *runs, green, passes, noColor, red, failures, noColor, progressBar, estimate)

		cmd := exec.Command("npm", npmArgs...)
		cmd.Dir = uiDir

		output, err := cmd.CombinedOutput()
		if *verbose {
			fmt.Println(string(output))
		}

		if err != nil {
			failures++
		} else {
			passes++
			if *perf {
				scanner := bufio.NewScanner(strings.NewReader(string(output)))
				parseTimings(scanner, testTimings, &allTimings)
			}
		}
	}

	fmt.Println()
	fmt.Println("Test check complete.")
	fmt.Printf("Result: %s%d passed%s, %s%d failed%s out of %d runs.\n", green, passes, noColor, red, failures, noColor, *runs)

	if *perf {
		printPerformanceMetrics(testTimings, allTimings)
	}

	if failures > 0 {
		os.Exit(1)
	}
}

func parseTimings(scanner *bufio.Scanner, testTimings map[string][]float64, allTimings *[]float64) {
	inSlowestExamples := false
	timeRegex := regexp.MustCompile(`([0-9.]+) seconds`)
	totalTimeRegex := regexp.MustCompile(`Time:\s+([0-9.]+)\s?s`)

	var currentTestName string
	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "Top") && strings.Contains(line, "slowest examples") {
			inSlowestExamples = true
			continue
		}
		if strings.Contains(line, "Test Suites:") {
			inSlowestExamples = false
			continue
		}

		if inSlowestExamples {
			if !strings.HasPrefix(line, " ") {
				continue
			}
			if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
				currentTestName = strings.TrimSpace(line)
			}

			if timeRegex.MatchString(line) && currentTestName != "" {
				matches := timeRegex.FindStringSubmatch(line)
				if len(matches) > 1 {
					timeVal, err := strconv.ParseFloat(matches[1], 64)
					if err == nil {
						testTimings[currentTestName] = append(testTimings[currentTestName], timeVal)
					}
				}
			}
		}

		if totalTimeRegex.MatchString(line) {
			matches := totalTimeRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				runTime, err := strconv.ParseFloat(matches[1], 64)
				if err == nil {
					*allTimings = append(*allTimings, runTime*1000) // convert to ms
				}
			}
		}
	}
}

func printPerformanceMetrics(testTimings map[string][]float64, allTimings []float64) {
	fmt.Printf("\n%sAverage timings for the slowest tests:%s\n", yellow, noColor)

	type testTiming struct {
		name    string
		avgTime float64
	}
	var sortedTimings []testTiming

	for name, timings := range testTimings {
		var total float64
		for _, t := range timings {
			total += t
		}
		avg := total / float64(len(timings))
		sortedTimings = append(sortedTimings, testTiming{name: name, avgTime: avg})
	}

	sort.Slice(sortedTimings, func(i, j int) bool {
		return sortedTimings[i].avgTime > sortedTimings[j].avgTime
	})

	for _, timing := range sortedTimings {
		fmt.Printf("%.3fs\t%s\n", timing.avgTime, timing.name)
	}

	if len(allTimings) > 0 {
		fmt.Printf("\n%sOverall test performance metrics:%s\n", yellow, noColor)

		var total float64
		for _, t := range allTimings {
			total += t
		}
		avgTimeMs := total / float64(len(allTimings))
		fmt.Printf("Average: %.3fs\n", avgTimeMs/1000)

		sort.Float64s(allTimings)

		p50 := percentile(allTimings, 50)
		p90 := percentile(allTimings, 90)
		p99 := percentile(allTimings, 99)

		fmt.Printf("p50 (median): %.3fs\n", p50/1000)
		fmt.Printf("p90: %.3fs\n", p90/1000)
		fmt.Printf("p99: %.3fs\n", p99/1000)
	}
}

func percentile(data []float64, perc int) float64 {
	if len(data) == 0 {
		return 0
	}
	index := (float64(perc) / 100) * float64(len(data)-1)
	i := int(index)
	if i >= len(data)-1 {
		return data[len(data)-1]
	}
	frac := index - float64(i)
	return data[i] + frac*(data[i+1]-data[i])
}
