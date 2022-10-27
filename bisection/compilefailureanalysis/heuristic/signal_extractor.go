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

package heuristic

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util"
)

const (
	// Patterns for Python stack trace frames.
	PYTHON_STACK_TRACE_FRAME_PATTERN_1 = `File "(?P<file>.+\.py)", line (?P<line>[0-9]+), in (?P<function>.+)`
	PYTHON_STACK_TRACE_FRAME_PATTERN_2 = `(?P<function>[^\s]+) at (?P<file>.+\.py):(?P<line>[0-9]+)`
	// Match file path separator: "/", "//", "\", "\\".
	PATH_SEPARATOR_PATTERN = `(?:/{1,2}|\\{1,2})`

	// Match drive root directory on Windows, like "C:/" or "C:\\".
	WINDOWS_ROOT_PATTERN = `[a-zA-Z]:` + PATH_SEPARATOR_PATTERN

	// Match system root directory on Linux/Mac.
	UNIX_ROOT_PATTERN = `/+`

	// Match system/drive root on Linux/Mac/Windows.
	ROOT_DIR_PATTERN = "(?:" + WINDOWS_ROOT_PATTERN + "|" + UNIX_ROOT_PATTERN + ")"

	// Match file/directory names and also match ., ..
	FILE_NAME_PATTERN = `[\w\.-]+`

	// Mark the beginning of the failure section in stdout log
	FAILURE_SECTION_START_PREFIX = "FAILED: "

	// Mark the end of the failure section in stdout log
	FAILURE_SECTION_END_PATTERN_1 = `^\d+ errors? generated.`
	FAILURE_SECTION_END_PATTERN_2 = `failed with exit code \d+`
	// If it reads this line, it is also ends of failure section
	OUTSIDE_FAILURE_SECTION_PATTERN = `\[\d+/\d+\]`

	NINJA_FAILURE_LINE_END_PREFIX = `ninja: build stopped`
	NINJA_ERROR_LINE_PREFIX       = `ninja: error`

	STDLOG_NODE_PATTERN = `(?:"([^"]+)")|(\S+)`
)

// ExtractSignals extracts necessary signals for heuristic analysis from logs
func ExtractSignals(c context.Context, compileLogs *model.CompileLogs) (*model.CompileFailureSignal, error) {
	if compileLogs.NinjaLog == nil && compileLogs.StdOutLog == "" {
		return nil, fmt.Errorf("Unable to extract signals from empty logs.")
	}
	// Prioritise extracting signals from ninja logs instead of stdout logs
	if compileLogs.NinjaLog != nil {
		return ExtractSignalsFromNinjaLog(c, compileLogs.NinjaLog)
	}
	return ExtractSignalsFromStdoutLog(c, compileLogs.StdOutLog)
}

// ExtractSignalsFromNinjaLog extracts necessary signals for heuristic analysis from ninja log
func ExtractSignalsFromNinjaLog(c context.Context, ninjaLog *model.NinjaLog) (*model.CompileFailureSignal, error) {
	signal := &model.CompileFailureSignal{}
	for _, failure := range ninjaLog.Failures {
		edge := &model.CompileFailureEdge{
			Rule:         failure.Rule,
			OutputNodes:  failure.OutputNodes,
			Dependencies: normalizeDependencies(failure.Dependencies),
		}
		signal.Edges = append(signal.Edges, edge)
		signal.Nodes = append(signal.Nodes, failure.OutputNodes...)
		e := extractFiles(signal, failure.Output)
		if e != nil {
			return nil, e
		}
	}
	return signal, nil
}

func extractFiles(signal *model.CompileFailureSignal, output string) error {
	pythonPatterns := []*regexp.Regexp{
		regexp.MustCompile(PYTHON_STACK_TRACE_FRAME_PATTERN_1),
		regexp.MustCompile(PYTHON_STACK_TRACE_FRAME_PATTERN_2),
	}
	filePathLinePattern := regexp.MustCompile(getFileLinePathPatternStr())

	lines := strings.Split(output, "\n")
	for i, line := range lines {
		// Do not extract the first line
		if i == 0 {
			continue
		}
		// Check if the line matches python pattern
		matchedPython := false
		for _, pythonPattern := range pythonPatterns {
			matches, err := util.MatchedNamedGroup(pythonPattern, line)
			if err == nil {
				pyLine, e := strconv.Atoi(matches["line"])
				if e != nil {
					return e
				}
				signal.AddLine(util.NormalizeFilePath(matches["file"]), pyLine)
				matchedPython = true
				continue
			}
		}
		if matchedPython {
			continue
		}
		// Non-python cases
		matches := filePathLinePattern.FindAllStringSubmatch(line, -1)
		if matches != nil {
			for _, match := range matches {
				if len(match) != 3 {
					return fmt.Errorf("Invalid line: %s", line)
				}
				// match[1] is file, match[2] is line number
				if match[2] == "" {
					signal.AddFilePath(util.NormalizeFilePath(match[1]))
				} else {
					lineInt, e := strconv.Atoi(match[2])
					if e != nil {
						return e
					}
					signal.AddLine(util.NormalizeFilePath(match[1]), lineInt)
				}
			}
		}
	}
	return nil
}

func extractFilesFromLine(signal *model.CompileFailureSignal, line string) error {
	pythonPatterns := []*regexp.Regexp{
		regexp.MustCompile(PYTHON_STACK_TRACE_FRAME_PATTERN_1),
		regexp.MustCompile(PYTHON_STACK_TRACE_FRAME_PATTERN_2),
	}
	filePathLinePattern := regexp.MustCompile(getFileLinePathPatternStr())

	// Check if the line matches python pattern
	matchedPython := false
	for _, pythonPattern := range pythonPatterns {
		matches, err := util.MatchedNamedGroup(pythonPattern, line)
		if err == nil {
			pyLine, e := strconv.Atoi(matches["line"])
			if e != nil {
				return e
			}
			signal.AddLine(util.NormalizeFilePath(matches["file"]), pyLine)
			matchedPython = true
			continue
		}
	}
	if matchedPython {
		return nil
	}
	// Non-python cases
	matches := filePathLinePattern.FindAllStringSubmatch(line, -1)
	if matches != nil {
		for _, match := range matches {
			if len(match) != 3 {
				return fmt.Errorf("Invalid line: %s", line)
			}
			// match[1] is file, match[2] is line number
			if match[2] == "" {
				signal.AddFilePath(util.NormalizeFilePath(match[1]))
			} else {
				lineInt, e := strconv.Atoi(match[2])
				if e != nil {
					return e
				}
				signal.AddLine(util.NormalizeFilePath(match[1]), lineInt)
			}
		}
	}
	return nil
}

func normalizeDependencies(dependencies []string) []string {
	result := []string{}
	for _, dependency := range dependencies {
		result = append(result, util.NormalizeFilePath(dependency))
	}
	return result
}

// ExtractSignalsFromStdoutLog extracts necessary signals for heuristic analysis from stdout log
func ExtractSignalsFromStdoutLog(c context.Context, stdoutLog string) (*model.CompileFailureSignal, error) {
	signal := &model.CompileFailureSignal{}
	lines := strings.Split(stdoutLog, "\n")
	failureSectionEndPattern1 := regexp.MustCompile(FAILURE_SECTION_END_PATTERN_1)
	failureSectionEndPattern2 := regexp.MustCompile(FAILURE_SECTION_END_PATTERN_2)
	outsideFailureSectionPattern := regexp.MustCompile(OUTSIDE_FAILURE_SECTION_PATTERN)
	failureStarted := false
	for _, line := range lines {
		line = strings.Trim(line, " \t")
		if strings.HasPrefix(line, FAILURE_SECTION_START_PREFIX) {
			failureStarted = true
			line = line[len(FAILURE_SECTION_START_PREFIX):]
			signal.Nodes = append(signal.Nodes, extractNodes(line)...)
			continue
		} else if failureStarted && strings.HasPrefix(line, NINJA_FAILURE_LINE_END_PREFIX) {
			// End parsing
			break
		} else if failureStarted && (failureSectionEndPattern1.MatchString(line) || failureSectionEndPattern2.MatchString(line) || outsideFailureSectionPattern.MatchString(line)) {
			failureStarted = false
		}

		if failureStarted || strings.HasPrefix(line, NINJA_ERROR_LINE_PREFIX) {
			extractFilesFromLine(signal, line)
		}
	}
	return signal, nil
}

// extractNode returns the list of failed output nodes.
// Possible format:
// FAILED: obj/path/to/file.o
// FAILED: target.exe
// FAILED: "target with space in name"
func extractNodes(line string) []string {
	pattern := regexp.MustCompile(STDLOG_NODE_PATTERN)
	matches := pattern.FindAllStringSubmatch(line, -1)
	result := []string{}
	for _, match := range matches {
		for i := 1; i <= 2; i++ {
			if match[i] != "" {
				result = append(result, match[i])
			}
		}
	}
	return result
}

// getFileLinePathPatternStr matches a full file path and line number.
// It could match files with or without line numbers like below:
//
//	c:\\a\\b.txt:12
//	c:\a\b.txt(123)
//	c:\a\b.txt:[line 123]
//	D:/a/b.txt
//	/a/../b/./c.txt
//	a/b/c.txt
//	//BUILD.gn:246
func getFileLinePathPatternStr() string {
	pattern := `(`
	pattern += ROOT_DIR_PATTERN + "?"                                    // System/Drive root directory.
	pattern += `(?:` + FILE_NAME_PATTERN + PATH_SEPARATOR_PATTERN + `)*` // Directories.
	pattern += FILE_NAME_PATTERN + `\.` + getFileExtensionPatternStr()
	pattern += `)`                           // File name and extension.
	pattern += `(?:(?:[\(:]|\[line )(\d+))?` // Line number might not be available.
	return pattern
}

// getFileExtensionPattern matches supported file extensions.
// Sort extension list to avoid non-full match like 'c' matching 'c' in 'cpp'.
func getFileExtensionPatternStr() string {
	extensions := getSupportedFileExtension()
	sort.Sort(sort.Reverse(sort.StringSlice(extensions)))
	return fmt.Sprintf("(?:%s)", strings.Join(extensions, "|"))
}

// getSupportedFileExtension get gile extensions to filter out files from log.
func getSupportedFileExtension() []string {
	return []string{
		"c",
		"cc",
		"cpp",
		"css",
		"exe",
		"gn",
		"gni",
		"gyp",
		"gypi",
		"h",
		"hh",
		"html",
		"idl",
		"isolate",
		"java",
		"js",
		"json",
		"m",
		"mm",
		"mojom",
		"nexe",
		"o",
		"obj",
		"py",
		"pyc",
		"rc",
		"sh",
		"sha1",
		"ts",
		"txt",
	}
}
