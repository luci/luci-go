// Copyright 2026 The LUCI Authors.
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

package application

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/spec"
	"go.chromium.org/luci/vpython/standard"
	"go.chromium.org/luci/vpython/standard/legacy"
)

// UpgradeSpecs converts legacy specs in target path to standard formats,
// or force-regenerates lockfiles for standard specs.
func UpgradeSpecs(ctx context.Context, path string, keepLegacy, legacyMode, allMode bool) error {
	absPath := path
	if err := filesystem.AbsPath(&absPath); err != nil {
		return errors.Fmt("failed to get absolute target path of %q: %w", path, err)
	}

	st, err := os.Stat(absPath)
	if err != nil {
		return errors.Fmt("target path %q does not exist: %w", path, err)
	}

	if legacyMode || allMode {
		if st.IsDir() {
			if legacyMode {
				return upgradeDir(ctx, absPath, keepLegacy)
			}
			return forceRegenerateDir(ctx, absPath)
		}
		if legacyMode {
			return upgradeFile(ctx, absPath, keepLegacy, true)
		}
		return forceRegenerateFile(ctx, absPath, true)
	}

	// Git-based modified files detection (default mode)
	if st.IsDir() {
		modifiedFiles, err := detectGitModifiedFiles(ctx, absPath)
		if err != nil {
			return err
		}
		if len(modifiedFiles) == 0 {
			fmt.Printf("vpython3 upgrade: No Git-modified vpython specs or Python scripts detected in %s. (Use -all to scan/compile all files recursively).\n", path)
			return nil
		}

		var errs errors.MultiError
		for _, file := range modifiedFiles {
			if legacyMode {
				if errFile := upgradeFile(ctx, file, keepLegacy, false); errFile != nil {
					errs = append(errs, errFile)
				}
			} else {
				if errFile := forceRegenerateFile(ctx, file, false); errFile != nil {
					errs = append(errs, errFile)
				}
			}
		}
		return errs.AsError()
	}

	// Direct single file target
	if legacyMode {
		return upgradeFile(ctx, absPath, keepLegacy, true)
	}
	return forceRegenerateFile(ctx, absPath, true)
}

func upgradeDir(ctx context.Context, dirPath string, keepLegacy bool) error {
	var errs errors.MultiError
	errWalk := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			// Skip symbolic links to prevent unexpected target file mutations or orphaning.
			return nil
		}
		if d.IsDir() {
			// Skip standard hidden folders.
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." && d.Name() != ".." {
				return filepath.SkipDir
			}
			return nil
		}
		if errFile := upgradeFile(ctx, path, keepLegacy, false); errFile != nil {
			errs = append(errs, errFile)
		}
		return nil
	})

	if errWalk != nil {
		errs = append(errs, errWalk)
	}

	return errs.AsError()
}

func upgradeFile(ctx context.Context, filePath string, keepLegacy bool, isDirectTarget bool) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".vpython" || ext == ".vpython3" {
		return convertStandaloneSpec(ctx, filePath, keepLegacy, isDirectTarget)
	}

	isPy, err := isPythonScript(filePath)
	if err != nil {
		if isDirectTarget {
			return err
		}
		return nil
	}

	if isPy {
		return convertInlineSpec(ctx, filePath)
	}

	if isDirectTarget {
		logging.Warningf(ctx, "Skipped unsupported file target %q. The specs upgrade tool only supports migrating standalone specifications (.vpython, .vpython3) and Python scripts.", filePath)
	}
	return nil
}

func marshalProjectSpec(spec *standard.ProjectSpec) (string, error) {
	schema := &standard.ProjectSpec{
		RequiresPython: spec.RequiresPython,
		Dependencies:   spec.Dependencies,
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentSymbol("  ")
	enc.SetArraysMultiline(true)
	if err := enc.Encode(schema); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func marshalScriptMetadataSpec(spec *standard.ProjectSpec) (string, error) {
	schema := &standard.ProjectSpec{
		RequiresPython: spec.RequiresPython,
		Dependencies:   spec.Dependencies,
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentSymbol("  ")
	enc.SetArraysMultiline(true)
	if err := enc.Encode(schema); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func convertStandaloneSpec(ctx context.Context, srcPath string, keepLegacy bool, isDirectTarget bool) error {
	// Check if this standalone spec is a script companion spec.
	dir := filepath.Dir(srcPath)
	base := filepath.Base(srcPath)
	var scriptPath string

	if before, found := strings.CutSuffix(base, ".vpython3"); found && before != "" {
		if strings.HasSuffix(before, ".py") {
			scriptPath = filepath.Join(dir, before)
		} else {
			cand1 := filepath.Join(dir, before+".py")
			cand2 := filepath.Join(dir, before)
			if st, err := os.Stat(cand1); err == nil && !st.IsDir() {
				if ok, _ := isPythonScript(cand1); ok {
					scriptPath = cand1
				}
			} else if st, err := os.Stat(cand2); err == nil && !st.IsDir() {
				if ok, _ := isPythonScript(cand2); ok {
					scriptPath = cand2
				}
			}
		}
	} else if before, found := strings.CutSuffix(base, ".vpython"); found && before != "" {
		if strings.HasSuffix(before, ".py") {
			scriptPath = filepath.Join(dir, before)
		} else {
			cand1 := filepath.Join(dir, before+".py")
			cand2 := filepath.Join(dir, before)
			if st, err := os.Stat(cand1); err == nil && !st.IsDir() {
				if ok, _ := isPythonScript(cand1); ok {
					scriptPath = cand1
				}
			} else if st, err := os.Stat(cand2); err == nil && !st.IsDir() {
				if ok, _ := isPythonScript(cand2); ok {
					scriptPath = cand2
				}
			}
		}
	}

	if scriptPath != "" {
		// Merge companion spec inline.
		return mergeSpecIntoScript(ctx, scriptPath, srcPath, keepLegacy)
	}

	if base != ".vpython" && base != ".vpython3" {
		if isDirectTarget {
			logging.Warningf(ctx, "Skipped custom spec file %q. Standalone custom spec files cannot be converted to folder-level vpython.toml as they represent explicitly-called environments. Only standard .vpython or .vpython3 common spec files are converted.", srcPath)
		}
		return nil
	}

	if base == ".vpython" {
		// Bypassed legacy .vpython migration if .vpython3 exists due to precedence.
		vpython3Path := filepath.Join(dir, ".vpython3")
		if _, err := os.Stat(vpython3Path); err == nil {
			if !keepLegacy {
				// Clean up obsolete legacy .vpython file to protect workspace cleanliness!
				if err := os.Remove(srcPath); err != nil {
					return errors.Fmt("failed to delete obsolete legacy .vpython file %q: %w", srcPath, err)
				}
				logging.Infof(ctx, "Cleaned up obsolete legacy .vpython file bypassed by .vpython3 precedence: %s", srcPath)
			}
			return nil
		}
	}

	var sp vpython.Spec
	if err := spec.Load(srcPath, &sp); err != nil {
		return errors.Fmt("failed to load legacy spec file %q: %w", srcPath, err)
	}

	projectSpec, err := legacy.TranslateLegacySpec(&sp)
	if err != nil {
		return errors.Fmt("failed to translate legacy spec %q: %w", srcPath, err)
	}

	tomlContent, err := marshalProjectSpec(projectSpec)
	if err != nil {
		return errors.Fmt("failed to serialize TOML for spec %q: %w", srcPath, err)
	}

	destPath := filepath.Join(filepath.Dir(srcPath), "vpython.toml")
	var perm os.FileMode = 0644
	if st, err := os.Stat(destPath); err == nil && !st.IsDir() {
		perm = st.Mode().Perm()
	}

	tmpPath := destPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(tomlContent), perm); err != nil {
		return errors.Fmt("failed to write temporary vpython.toml %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return errors.Fmt("failed to replace vpython.toml %q: %w", destPath, err)
	}

	// Best-effort proactive lockfile generation.
	generateProactiveLockfile(ctx, destPath, projectSpec.Dependencies)
	if !keepLegacy {
		if err := os.Remove(srcPath); err != nil {
			return errors.Fmt("failed to delete legacy spec file %q after migration: %w", srcPath, err)
		}
		fmt.Printf("Successfully migrated legacy spec: %s -> %s\n", srcPath, destPath)
		logging.Infof(ctx, "Successfully migrated legacy spec: %s -> %s", srcPath, destPath)
	} else {
		fmt.Printf("Successfully migrated legacy spec (kept original): %s -> %s\n", srcPath, destPath)
		logging.Infof(ctx, "Successfully migrated legacy spec (kept original): %s -> %s", srcPath, destPath)
	}
	return nil
}

func mergeSpecIntoScript(ctx context.Context, scriptPath, specPath string, keepLegacy bool) error {
	var sp vpython.Spec
	if err := spec.Load(specPath, &sp); err != nil {
		return errors.Fmt("failed to load legacy spec file %q: %w", specPath, err)
	}

	projectSpec, err := legacy.TranslateLegacySpec(&sp)
	if err != nil {
		return errors.Fmt("failed to translate legacy spec %q: %w", specPath, err)
	}

	tomlContent, err := marshalScriptMetadataSpec(projectSpec)
	if err != nil {
		return err
	}

	tomlLines := strings.Split(strings.TrimSpace(tomlContent), "\n")
	var pep723Block [][]byte
	pep723Block = append(pep723Block, []byte("# /// script"))
	for _, tl := range tomlLines {
		if tl == "" {
			pep723Block = append(pep723Block, []byte("#"))
		} else {
			pep723Block = append(pep723Block, []byte("# "+tl))
		}
	}
	pep723Block = append(pep723Block, []byte("# ///"))

	st, err := os.Stat(scriptPath)
	if err != nil {
		return errors.Fmt("failed to stat script %q: %w", scriptPath, err)
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return errors.Fmt("failed to read script %q: %w", scriptPath, err)
	}
	lines := bytes.Split(content, []byte{'\n'})

	hasPep723 := false
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("#")) && bytes.Contains(trimmed, []byte("/// script")) {
			hasPep723 = true
			break
		}
	}

	if hasPep723 {
		logging.Warningf(ctx, "Script %q already contains a standard PEP 723 shebang block! Skipping automated companion spec merging from %q.", scriptPath, specPath)
		return nil
	}

	beginIdx := -1
	endIdx := -1
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("#")) && bytes.Contains(trimmed, []byte(spec.DefaultInlineBeginGuard)) {
			beginIdx = i
		} else if bytes.HasPrefix(trimmed, []byte("#")) && bytes.Contains(trimmed, []byte(spec.DefaultInlineEndGuard)) {
			endIdx = i
			break
		}
	}

	if (beginIdx != -1 && endIdx == -1) || (beginIdx == -1 && endIdx != -1) {
		return errors.Fmt("script %q contains mismatched inline spec guards (begin index: %d, end index: %d)", scriptPath, beginIdx, endIdx)
	}

	var newLines [][]byte
	if beginIdx != -1 && endIdx != -1 {
		newLines = make([][]byte, 0, len(lines)-(endIdx-beginIdx+1)+len(pep723Block))
		newLines = append(newLines, lines[:beginIdx]...)
		newLines = append(newLines, pep723Block...)
		newLines = append(newLines, lines[endIdx+1:]...)
	} else {
		insertIdx := 0
		if len(lines) > 0 && bytes.HasPrefix(bytes.TrimSpace(lines[0]), []byte("#!")) {
			insertIdx = 1
		}
		newLines = make([][]byte, 0, len(lines)+len(pep723Block)+1)
		newLines = append(newLines, lines[:insertIdx]...)
		newLines = append(newLines, pep723Block...)
		newLines = append(newLines, []byte(""))
		newLines = append(newLines, lines[insertIdx:]...)
	}

	tmpPath := scriptPath + ".tmp"
	if err := os.WriteFile(tmpPath, bytes.Join(newLines, []byte{'\n'}), st.Mode().Perm()); err != nil {
		return errors.Fmt("failed to write temporary script %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, scriptPath); err != nil {
		_ = os.Remove(tmpPath)
		return errors.Fmt("failed to replace script %q: %w", scriptPath, err)
	}

	if !keepLegacy {
		if err := os.Remove(specPath); err != nil {
			return errors.Fmt("failed to delete companion spec file %q after merging: %w", specPath, err)
		}
		fmt.Printf("Successfully merged companion spec: %s -> injected inline inside script %s\n", specPath, scriptPath)
		logging.Infof(ctx, "Successfully merged companion spec: %s -> injected inline inside script %s", specPath, scriptPath)
	} else {
		fmt.Printf("Successfully merged companion spec: %s -> injected inline inside script %s (kept original spec file)\n", specPath, scriptPath)
		logging.Infof(ctx, "Successfully merged companion spec: %s -> injected inline inside script %s (kept original spec file)", specPath, scriptPath)
	}
	// Best-effort proactive lockfile generation.
	generateProactiveLockfile(ctx, scriptPath, projectSpec.Dependencies)
	return nil
}

func convertInlineSpec(ctx context.Context, scriptPath string) error {
	// If a higher-precedence companion spec file exists in the same directory,
	// we strictly skip converting the lower-precedence inline spec block!
	// The companion spec migration will process next and cleanly overwrite this block.
	if hasCompanionSpecFile(scriptPath) {
		return nil
	}

	fd, err := os.Open(scriptPath)
	if err != nil {
		return errors.Fmt("failed to read script %q: %w", scriptPath, err)
	}
	defer fd.Close()

	beginGuard := []byte(spec.DefaultInlineBeginGuard)
	endGuard := []byte(spec.DefaultInlineEndGuard)

	// Fast-path line-by-line scanning: stop as soon as we hit executable code.
	hasInlineSpec := false
	s := bufio.NewScanner(fd)
	for s.Scan() {
		trimmed := bytes.TrimSpace(s.Bytes())
		if len(trimmed) == 0 {
			continue
		}
		if !bytes.HasPrefix(trimmed, []byte("#")) {
			break
		}
		if bytes.Contains(trimmed, beginGuard) || bytes.Contains(trimmed, endGuard) {
			hasInlineSpec = true
			break
		}
	}
	if err := s.Err(); err != nil {
		return err
	}

	if !hasInlineSpec {
		return nil
	}

	fd.Close()

	st, err := os.Stat(scriptPath)
	if err != nil {
		return errors.Fmt("failed to stat script %q: %w", scriptPath, err)
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return errors.Fmt("failed to read script %q: %w", scriptPath, err)
	}
	lines := bytes.Split(content, []byte{'\n'})

	beginIdx := -1
	endIdx := -1
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("#")) && bytes.Contains(trimmed, beginGuard) {
			beginIdx = i
		} else if bytes.HasPrefix(trimmed, []byte("#")) && bytes.Contains(trimmed, endGuard) {
			endIdx = i
			break
		}
	}

	if (beginIdx != -1 && endIdx == -1) || (beginIdx == -1 && endIdx != -1) {
		return errors.Fmt("script %q contains mismatched inline spec guards (begin index: %d, end index: %d)", scriptPath, beginIdx, endIdx)
	}

	if beginIdx == -1 || endIdx == -1 {
		return nil
	}

	// Check if standard shebang is present.
	hasPep723 := false
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("#")) && bytes.Contains(trimmed, []byte("/// script")) {
			hasPep723 = true
			break
		}
	}

	var specLines [][]byte
	for i := beginIdx + 1; i < endIdx; i++ {
		trimmed := bytes.TrimSpace(lines[i])
		if bytes.HasPrefix(trimmed, []byte("#")) {
			stripped := trimmed[1:]
			leftTrimmed := bytes.TrimLeft(stripped, " \t")
			specLines = append(specLines, leftTrimmed)
		}
	}

	var sp vpython.Spec
	joined := bytes.Join(specLines, []byte{'\n'})
	if err := spec.Parse(string(joined), &sp); err != nil {
		return errors.Fmt("failed to parse inline legacy spec inside %q: %w", scriptPath, err)
	}

	if strings.HasPrefix(sp.PythonVersion, "2") {
		// Python 2 inline specs are ignored. PEP 723 is a modern Python 3 era
		// standard, and inline metadata comments are not supported by the legacy
		// Python 2.7 vpython launcher.
		return nil
	}

	projectSpec, err := legacy.TranslateLegacySpec(&sp)
	if err != nil {
		return errors.Fmt("failed to translate inline legacy spec inside %q: %w", scriptPath, err)
	}

	tomlContent, err := marshalScriptMetadataSpec(projectSpec)
	if err != nil {
		return err
	}

	tomlLines := strings.Split(strings.TrimSpace(tomlContent), "\n")
	var pep723Block [][]byte
	pep723Block = append(pep723Block, []byte("# /// script"))
	for _, tl := range tomlLines {
		if tl == "" {
			pep723Block = append(pep723Block, []byte("#"))
		} else {
			pep723Block = append(pep723Block, []byte("# "+tl))
		}
	}
	pep723Block = append(pep723Block, []byte("# ///"))

	if hasPep723 {
		fmt.Fprintf(os.Stderr, "Warning: Script %q already contains a standard PEP 723 shebang block! Skipping automated conversion.\n", scriptPath)
		logging.Warningf(ctx, "Script %q already contains a standard PEP 723 shebang block! Skipping automated conversion.", scriptPath)
		return nil
	}

	// Replace legacy block in-place.
	newLines := make([][]byte, 0, len(lines)-(endIdx-beginIdx+1)+len(pep723Block))
	newLines = append(newLines, lines[:beginIdx]...)
	newLines = append(newLines, pep723Block...)
	newLines = append(newLines, lines[endIdx+1:]...)

	tmpPath := scriptPath + ".tmp"
	if err := os.WriteFile(tmpPath, bytes.Join(newLines, []byte{'\n'}), st.Mode().Perm()); err != nil {
		return errors.Fmt("failed to write temporary script %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, scriptPath); err != nil {
		_ = os.Remove(tmpPath)
		return errors.Fmt("failed to replace script %q: %w", scriptPath, err)
	}

	fmt.Printf("Successfully migrated inline legacy spec -> PEP 723 shebang in script %q\n", scriptPath)
	logging.Infof(ctx, "Successfully migrated inline legacy spec -> PEP 723 shebang in script %q", scriptPath)
	// Best-effort proactive lockfile generation.
	generateProactiveLockfile(ctx, scriptPath, projectSpec.Dependencies)
	return nil
}

func isPythonScript(path string) (bool, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".py" {
		return true, nil
	}

	// If no .py extension, perform strict safety checks to prevent binary corruption!
	fd, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer fd.Close()

	buf := make([]byte, 1024)
	n, err := fd.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}
	buf = buf[:n]

	// 1. Null-byte check (binary detection).
	if bytes.Contains(buf, []byte{0}) {
		return false, nil
	}

	// 2. Python shebang check.
	idx := bytes.IndexByte(buf, '\n')
	if idx == -1 {
		idx = len(buf)
	}
	firstLine := bytes.TrimSpace(buf[:idx])
	if !bytes.HasPrefix(firstLine, []byte("#!")) {
		return false, nil
	}
	lower := bytes.ToLower(firstLine)
	if bytes.Contains(lower, []byte("python")) || bytes.Contains(lower, []byte("vpython")) {
		return true, nil
	}

	return false, nil
}

func hasCompanionSpecFile(scriptPath string) bool {
	dir := filepath.Dir(scriptPath)
	base := filepath.Base(scriptPath)

	// 1. Check <script>.vpython / <script>.vpython3
	path1 := scriptPath + ".vpython3"
	path2 := scriptPath + ".vpython"
	if st, err := os.Stat(path1); err == nil && !st.IsDir() {
		return true
	}
	if st, err := os.Stat(path2); err == nil && !st.IsDir() {
		return true
	}

	// 2. Check <script_no_ext>.vpython / <script_no_ext>.vpython3 (only if script ends in .py!)
	if strings.HasSuffix(base, ".py") {
		noExt := strings.TrimSuffix(base, ".py")
		path3 := filepath.Join(dir, noExt+".vpython3")
		path4 := filepath.Join(dir, noExt+".vpython")
		if st, err := os.Stat(path3); err == nil && !st.IsDir() {
			return true
		}
		if st, err := os.Stat(path4); err == nil && !st.IsDir() {
			return true
		}
	}

	return false
}

func generateProactiveLockfile(ctx context.Context, specPath string, dependencies []string) {
	if len(dependencies) == 0 {
		return
	}
	uvBin := os.Getenv("VPYTHON_UV_BIN")
	if uvBin == "" {
		var err error
		uvBin, err = exec.LookPath("uv")
		if err != nil {
			logging.Infof(ctx, "Proactive lockfile skipped: 'uv' not found on PATH and VPYTHON_UV_BIN not set")
			return
		}
	}
	arURL := os.Getenv("VPYTHON_AR_URL")

	lockPath := specPath + ".uv.lock"
	msg := fmt.Sprintf("Automatically generating lockfile %s...", filepath.Base(lockPath))
	fmt.Println(msg)
	logging.Infof(ctx, "%s", msg)

	var reqs bytes.Buffer
	for _, dep := range dependencies {
		reqs.WriteString(dep)
		reqs.WriteByte('\n')
	}

	cmdLock := exec.CommandContext(ctx, uvBin, "pip", "compile", "-",
		"--universal",
		"--generate-hashes",
		"--no-header",
	)
	cmdLock.Dir = filepath.Dir(specPath)
	cmdLock.Stdin = &reqs
	env := append(os.Environ(),
		"UV_PYTHON_DOWNLOADS=never",
		"UV_NO_WORKSPACE=1",
	)
	if arURL != "" {
		env = append(env, "UV_DEFAULT_INDEX="+arURL)
	}
	cmdLock.Env = env

	var stdout, stderr bytes.Buffer
	cmdLock.Stdout = &stdout
	cmdLock.Stderr = &stderr

	if errCmd := cmdLock.Run(); errCmd != nil {
		errMsg := fmt.Sprintf("Warning: failed to automatically generate lockfile %s: %v\nOutput:\n%s", filepath.Base(lockPath), errCmd, stderr.String())
		fmt.Fprintln(os.Stderr, errMsg)
		logging.Warningf(ctx, "%s", errMsg)
	} else {
		if err := os.WriteFile(lockPath, stdout.Bytes(), 0644); err != nil {
			errMsg := fmt.Sprintf("Warning: failed to write lockfile %s: %v", filepath.Base(lockPath), err)
			fmt.Fprintln(os.Stderr, errMsg)
			logging.Warningf(ctx, "%s", errMsg)
		} else {
			successMsg := fmt.Sprintf("Successfully generated lockfile %s!", filepath.Base(lockPath))
			fmt.Println(successMsg)
			logging.Infof(ctx, "%s", successMsg)
		}
	}
}

func detectGitModifiedFiles(ctx context.Context, targetDir string) ([]string, error) {
	// Find Git repository top-level directory
	cmdRoot := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmdRoot.Dir = targetDir
	rootBytes, err := cmdRoot.Output()
	if err != nil {
		return nil, errors.Fmt("failed to find git repository root for target %s: %w", targetDir, err)
	}
	gitRoot := strings.TrimSpace(string(rootBytes))

	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, err
	}

	fileSet := make(map[string]struct{})

	// 1. Get porcelain status relative to git root (uncommitted & untracked files)
	cmdStatus := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmdStatus.Dir = gitRoot
	statusBytes, err := cmdStatus.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(statusBytes))
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) < 4 {
				continue
			}
			status := line[:2]
			// Ignore deleted files
			if strings.Contains(status, "D") {
				continue
			}
			relPath := line[3:]
			if strings.HasPrefix(relPath, "\"") && strings.HasSuffix(relPath, "\"") {
				relPath = relPath[1 : len(relPath)-1]
			}
			fileSet[filepath.Join(gitRoot, relPath)] = struct{}{}
		}
	}

	// 2. Get branch diff changes against upstream main branch (locally committed files)
	cmdDiff := exec.CommandContext(ctx, "git", "diff", "--name-only", "origin/main...")
	cmdDiff.Dir = gitRoot
	diffBytes, err := cmdDiff.Output()
	if err != nil {
		// Fallback to master if main branch doesn't exist
		cmdDiffMaster := exec.CommandContext(ctx, "git", "diff", "--name-only", "origin/master...")
		cmdDiffMaster.Dir = gitRoot
		diffBytes, err = cmdDiffMaster.Output()
	}

	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(diffBytes))
		for scanner.Scan() {
			relPath := strings.TrimSpace(scanner.Text())
			if relPath != "" {
				fileSet[filepath.Join(gitRoot, relPath)] = struct{}{}
			}
		}
	}

	var modifiedFiles []string
	for absPath := range fileSet {
		// Only process existing files
		st, err := os.Stat(absPath)
		if err != nil || st.IsDir() {
			continue
		}

		// Only process files inside targetDir
		if !strings.HasPrefix(absPath, absTargetDir) {
			continue
		}

		// Only process supported files
		ext := strings.ToLower(filepath.Ext(absPath))
		base := filepath.Base(absPath)
		if ext == ".py" || ext == ".vpython" || ext == ".vpython3" || base == "vpython.toml" {
			modifiedFiles = append(modifiedFiles, absPath)
		}
	}

	return modifiedFiles, nil
}

func forceRegenerateLockfile(ctx context.Context, specPath string) error {
	var dependencies []string
	base := filepath.Base(specPath)

	if base == "vpython.toml" {
		spec, err := standard.ParseVpythonTOML(specPath)
		if err != nil {
			return errors.Fmt("failed to parse standard spec TOML: %w", err)
		}
		dependencies = spec.Dependencies
	} else {
		spec, err := standard.ParseScriptMetadata(specPath)
		if err != nil {
			return errors.Fmt("failed to parse PEP 723 script shebang: %w", err)
		}
		if spec == nil {
			// Skip python scripts without shebangs when force-locking!
			return nil
		}
		dependencies = spec.Dependencies
	}

	lockPath := specPath + ".uv.lock"
	if len(dependencies) == 0 {
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return errors.Fmt("failed to delete obsolete lockfile %s: %w", filepath.Base(lockPath), err)
		}
		return nil
	}

	msg := fmt.Sprintf("Forcing regeneration of lockfile %s...", filepath.Base(lockPath))
	fmt.Println(msg)
	logging.Infof(ctx, "%s", msg)

	uvBin := os.Getenv("VPYTHON_UV_BIN")
	if uvBin == "" {
		var err error
		uvBin, err = exec.LookPath("uv")
		if err != nil {
			return errors.New("failed to resolve 'uv' on PATH and VPYTHON_UV_BIN is not set")
		}
	}
	arURL := os.Getenv("VPYTHON_AR_URL")

	var reqs bytes.Buffer
	for _, dep := range dependencies {
		reqs.WriteString(dep)
		reqs.WriteByte('\n')
	}

	cmdLock := exec.CommandContext(ctx, uvBin, "pip", "compile", "-",
		"--universal",
		"--generate-hashes",
		"--no-header",
	)
	cmdLock.Dir = filepath.Dir(specPath)
	cmdLock.Stdin = &reqs
	env := append(os.Environ(),
		"UV_PYTHON_DOWNLOADS=never",
		"UV_NO_WORKSPACE=1",
	)
	if arURL != "" {
		env = append(env, "UV_DEFAULT_INDEX="+arURL)
	}
	cmdLock.Env = env

	var stdout, stderr bytes.Buffer
	cmdLock.Stdout = &stdout
	cmdLock.Stderr = &stderr

	if errCmd := cmdLock.Run(); errCmd != nil {
		return errors.Fmt("failed to compile lockfile %s: %s\nOutput:\n%s", filepath.Base(lockPath), errCmd, stderr.String())
	}

	if err := os.WriteFile(lockPath, stdout.Bytes(), 0644); err != nil {
		return errors.Fmt("failed to write lockfile %s: %w", filepath.Base(lockPath), err)
	}

	fmt.Printf("Successfully force-regenerated lockfile %s!\n", filepath.Base(lockPath))
	logging.Infof(ctx, "Successfully force-regenerated lockfile %s!", filepath.Base(lockPath))
	return nil
}

func forceRegenerateFile(ctx context.Context, filePath string, isDirectTarget bool) error {
	base := filepath.Base(filePath)

	if base == "vpython.toml" {
		return forceRegenerateLockfile(ctx, filePath)
	}

	isPy, err := isPythonScript(filePath)
	if err != nil {
		if isDirectTarget {
			return err
		}
		return nil
	}

	if isPy {
		return forceRegenerateLockfile(ctx, filePath)
	}

	if isDirectTarget {
		logging.Warningf(ctx, "Skipped unsupported file target %q for lockfile regeneration.", filePath)
	}
	return nil
}

func forceRegenerateDir(ctx context.Context, dirPath string) error {
	var errs errors.MultiError
	errWalk := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." && d.Name() != ".." {
				return filepath.SkipDir
			}
			return nil
		}
		if errFile := forceRegenerateFile(ctx, path, false); errFile != nil {
			errs = append(errs, errFile)
		}
		return nil
	})

	if errWalk != nil {
		errs = append(errs, errWalk)
	}

	return errs.AsError()
}
