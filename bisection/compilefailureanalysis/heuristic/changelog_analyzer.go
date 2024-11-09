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
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util"
)

// ScoringCriteria represents how we score in the heuristic analysis.
type ScoringCriteria struct {
	// The score if the suspect touched the same file in the failure log.
	TouchedSameFile int
	// The score if the suspect touched a related file to a file in the failure log.
	TouchedRelatedFile int
	// The score if the suspect touched the same file and the same line as in the failure log.
	TouchedSameLine int
}

// AnalyzeChangeLogs analyzes the changelogs based on the failure signals.
// Returns a dictionary that maps the commits and the result found.
func AnalyzeChangeLogs(c context.Context, signal *model.CompileFailureSignal, changelogs []*model.ChangeLog) (*model.HeuristicAnalysisResult, error) {
	result := &model.HeuristicAnalysisResult{}
	for _, changelog := range changelogs {
		justification, err := AnalyzeOneChangeLog(c, signal, changelog)
		commit := changelog.Commit
		if err != nil {
			logging.Errorf(c, "Error analyzing change log for commit %s. Error: %w", commit, err)
			continue
		}

		// We only care about the relevant CLs
		if justification.GetScore() <= 0 {
			continue
		}

		reviewUrl, err := changelog.GetReviewUrl()
		if err != nil {
			logging.Errorf(c, "Error getting review URL for commit: %s. Error: %w", commit, err)
			continue
		}
		reviewTitle, err := changelog.GetReviewTitle()
		if err != nil {
			// Just log the error from getting the review title - suspect should still be added
			logging.Errorf(c, "Error getting review title for commit: %s. Error: %w", commit, err)
		}
		result.AddItem(commit, reviewUrl, reviewTitle, justification)
	}
	result.Sort()
	return result, nil
}

// AnalyzeOneChangeLog analyzes one changelog(revision) and returns the
// justification of how likely that changelog is the culprit.
func AnalyzeOneChangeLog(c context.Context, signal *model.CompileFailureSignal, changelog *model.ChangeLog) (*model.SuspectJustification, error) {
	// TODO (crbug.com/1295566): check DEPs file as well, if the CL touches DEPs.
	// This is a nice-to-have feature, and is an edge case.
	justification := &model.SuspectJustification{}
	author := changelog.Author.Email
	for _, email := range getNonBlamableEmail() {
		if email == author {
			return &model.SuspectJustification{IsNonBlamable: true}, nil
		}
	}

	// Check files and line number extracted from output
	criteria := &ScoringCriteria{
		TouchedSameFile:    10,
		TouchedRelatedFile: 2,
		TouchedSameLine:    20,
	}
	for file, lines := range signal.Files {
		for _, diff := range changelog.ChangeLogDiffs {
			e := updateJustification(c, justification, file, lines, diff, criteria, model.JustificationType_FAILURELOG)
			if e != nil {
				return nil, e
			}
		}
	}

	// Check for dependency.
	criteria = &ScoringCriteria{
		TouchedSameFile:    2,
		TouchedRelatedFile: 1,
	}

	// Calculate the score for dependencies using the DependencyMap
	for _, diff := range changelog.ChangeLogDiffs {
		oldPathName := util.GetCanonicalFileName(diff.OldPath)
		newPathName := util.GetCanonicalFileName(diff.NewPath)
		// Only check the dependency if either the old file or new file exists in the map
		oldPathDeps, oldPathOk := signal.DependencyMap[oldPathName]
		newPathDeps, newPathOk := signal.DependencyMap[newPathName]
		if oldPathOk || newPathOk {
			// Only process modified files once
			deps := oldPathDeps
			if oldPathName != newPathName {
				deps = append(oldPathDeps, newPathDeps...)
			}
			for _, dep := range deps {
				e := updateJustification(c, justification, dep, []int{}, diff, criteria, model.JustificationType_DEPENDENCY)
				if e != nil {
					return nil, e
				}
			}
		}
	}

	justification.Sort()
	return justification, nil
}

func updateJustification(c context.Context, justification *model.SuspectJustification, fileInLog string, lines []int, diff model.ChangeLogDiff, criteria *ScoringCriteria, justificationType model.JustificationType) error {
	// TODO (crbug.com/1295566): In case of MODIFY, also query Gitiles for the
	// changed region and compared with lines. If they intersect, increase the score.
	// This may lead to a better score indicator.

	// Get the relevant file paths from CLs
	relevantFilePaths := []string{}
	switch diff.Type {
	case model.ChangeType_ADD, model.ChangeType_COPY, model.ChangeType_MODIFY:
		relevantFilePaths = append(relevantFilePaths, diff.NewPath)
	case model.ChangeType_RENAME:
		relevantFilePaths = append(relevantFilePaths, diff.NewPath, diff.OldPath)
	case model.ChangeType_DELETE:
		relevantFilePaths = append(relevantFilePaths, diff.OldPath)
	default:
		return fmt.Errorf("Unsupported diff type %s", diff.Type)
	}
	for _, filePath := range relevantFilePaths {
		score := 0
		reason := ""
		if IsSameFile(filePath, fileInLog) {
			score = criteria.TouchedSameFile
			reason = getReasonSameFile(filePath, diff.Type, justificationType)
		} else if IsRelated(filePath, fileInLog) {
			score = criteria.TouchedRelatedFile
			reason = getReasonRelatedFile(filePath, diff.Type, fileInLog, justificationType)
		}
		if score > 0 {
			justification.AddItem(score, filePath, reason, justificationType)
		}
	}
	return nil
}

func getReasonSameFile(filePath string, changeType model.ChangeType, justificationType model.JustificationType) string {
	m := getChangeTypeActionMap()
	action := m[string(changeType)]
	switch justificationType {
	case model.JustificationType_FAILURELOG:
		return fmt.Sprintf("The file \"%s\" was %s and it was in the failure log.", filePath, action)
	case model.JustificationType_DEPENDENCY:
		return fmt.Sprintf("The file \"%s\" was %s and it was in the dependency.", filePath, action)
	default:
		return ""
	}
}

func getReasonRelatedFile(filePath string, changeType model.ChangeType, relatedFile string, justificationType model.JustificationType) string {
	m := getChangeTypeActionMap()
	action := m[string(changeType)]
	switch justificationType {
	case model.JustificationType_FAILURELOG:
		return fmt.Sprintf("The file \"%s\" was %s. It was related to the file %s which was in the failure log.", filePath, action, relatedFile)
	case model.JustificationType_DEPENDENCY:
		return fmt.Sprintf("The file \"%s\" was %s. It was related to the dependency %s.", filePath, action, relatedFile)
	default:
		return ""
	}
}

func getChangeTypeActionMap() map[string]string {
	return map[string]string{
		model.ChangeType_ADD:    "added",
		model.ChangeType_COPY:   "copied",
		model.ChangeType_RENAME: "renamed",
		model.ChangeType_MODIFY: "modified",
		model.ChangeType_DELETE: "deleted",
	}
}

// IsSameFile makes the best effort in guessing if the file in the failure log
// is the same as the file in the changelog or not.
// Args:
// fullFilePath: Full path of a file committed to git repo.
// fileInLog: File path appearing in a failure log. It may or may not be a full path.
// Example:
// ("chrome/test/base/chrome_process_util.h", "base/chrome_process_util.h") -> True
// ("a/b/x.cc", "a/b/x.cc") -> True
// ("c/x.cc", "a/b/c/x.cc") -> False
func IsSameFile(fullFilePath string, fileInLog string) bool {
	// In some cases, fileInLog is prepended with "src/", we want a relative path to src/
	fileInLog = strings.TrimPrefix(fileInLog, "src/")
	if fileInLog == fullFilePath {
		return true
	}
	return strings.HasSuffix(fullFilePath, fmt.Sprintf("/%s", fileInLog))
}

// IsRelated checks if 2 files are related.
// Example:
// file.h <-> file_impl.cc
// x.h <-> x.cc
func IsRelated(fullFilePath string, fileInLog string) bool {
	filePathExt := strings.TrimPrefix(filepath.Ext(fullFilePath), ".")
	fileInLogExt := strings.TrimPrefix(filepath.Ext(fileInLog), ".")
	if !AreRelelatedExtensions(filePathExt, fileInLogExt) {
		return false
	}

	if strings.HasSuffix(fileInLog, ".o") || strings.HasSuffix(fileInLog, ".obj") {
		fileInLog = NormalizeObjectFilePath(fileInLog)
	}

	if IsSameFile(util.StripExtensionAndCommonSuffixFromFilePath(fullFilePath), util.StripExtensionAndCommonSuffixFromFilePath(fileInLog)) {
		return true
	}

	return false
}

// NormalizeObjectFilePath normalizes the file path to an c/c++ object file.
// During compile, a/b/c/file.cc in TARGET will be compiled into object file
// obj/a/b/c/TARGET.file.o, thus 'obj/' and TARGET need to be removed from path.
func NormalizeObjectFilePath(filePath string) string {
	if !(strings.HasSuffix(filePath, ".o") || strings.HasSuffix(filePath, ".obj")) {
		return filePath
	}
	filePath = strings.TrimPrefix(filePath, "obj/")
	dir := filepath.Dir(filePath)
	fileName := filepath.Base(filePath)
	parts := strings.Split(fileName, ".")
	if len(parts) == 3 {
		// Special cases for file.cc.obj and similar cases
		if parts[1] != "c" && parts[1] != "cc" && parts[1] != "cpp" && parts[1] != "m" && parts[1] != "mm" {
			fileName = fmt.Sprintf("%s.%s", parts[1], parts[2])
		}
	} else if len(parts) > 3 {
		fileName = strings.Join(parts[1:], ".")
	}
	if dir == "." {
		return fileName
	}
	return fmt.Sprintf("%s/%s", dir, fileName)
}

// AreRelelatedExtensions checks if 2 extensions are related
func AreRelelatedExtensions(ext1 string, ext2 string) bool {
	relations := [][]string{
		{"h", "hh", "c", "cc", "cpp", "m", "mm", "o", "obj"},
		{"py", "pyc"},
		{"gyp", "gypi"},
	}
	for _, group := range relations {
		found1 := false
		found2 := false
		for _, ext := range group {
			if ext == ext1 {
				found1 = true
			}
			if ext == ext2 {
				found2 = true
			}
		}
		if found1 && found2 {
			return true
		}
	}
	return false
}

// getNonBlamableEmail returns emails whose changes should never be flagged as culprits.
func getNonBlamableEmail() []string {
	return []string{"chrome-release-bot@chromium.org"}
}
