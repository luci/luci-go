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

package format

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"go.chromium.org/luci/client/cmd/luci/base"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// DiscoveredError represents an error discovered through the 3-layer discovery hierarchy:
// 1. WorkUnit and ancestor SummaryMarkdown
// 2. Tradefed test result XML (<Reason> element in test_result.xml, subprocess-test_result.xml, etc.)
// 3. Harness / Root Invocation SummaryMarkdown
type DiscoveredError struct {
	Source         string // e.g. "Work Unit summary", "Artifact subprocess-test_result.xml...", "Root invocation summary"
	SourceWorkUnit string // Resource name of the work unit or invocation where the error originated
	ArtifactID     string // Artifact ID where the error was found (if from artifact)
	ErrorName      string // e.g. "INSTRUMENTATION_NULL_METHOD"
	ErrorCode      string // e.g. "530002"
	Message        string // Failure message
	StackTrace     string // Extracted stack trace
	RawSummary     string // Markdown summary from WorkUnit / Invocation
}

// XMLReason represents a Tradefed XML <Reason> tag.
//
// TODO(b/547503348): Remove XML fallback parsing once TradeFed uploads module
// failure reasons directly to WorkUnit.SummaryMarkdown and marks WorkUnits FAILED.
type XMLReason struct {
	XMLName   xml.Name `xml:"Reason"`
	Message   string   `xml:"message,attr"`
	ErrorName string   `xml:"error_name,attr"`
	ErrorCode string   `xml:"error_code,attr"`
	Content   string   `xml:",chardata"`
}

// ParseTradefedXMLReason parses an XML stream and returns the first non-empty <Reason> element.
//
// TODO(b/547503348): Remove once TradeFed populates WorkUnit.SummaryMarkdown directly.
func ParseTradefedXMLReason(r io.Reader) (*XMLReason, error) {
	decoder := xml.NewDecoder(r)
	for {
		t, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if se, ok := t.(xml.StartElement); ok {
			if strings.EqualFold(se.Name.Local, "reason") {
				var reason XMLReason
				if err := decoder.DecodeElement(&reason, &se); err == nil {
					if reason.Message != "" || reason.ErrorName != "" || reason.ErrorCode != "" || strings.TrimSpace(reason.Content) != "" {
						if reason.Message == "" {
							reason.Message = strings.TrimSpace(reason.Content)
						}
						return &reason, nil
					}
				}
			}
		}
	}
	return nil, nil
}

func cleanAndSplitReasonMessage(rawMessage string) (msg, stackTrace string) {
	s := strings.ReplaceAll(rawMessage, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimSpace(s)

	if idx := strings.Index(s, "Stack:"); idx != -1 {
		msg = strings.TrimSpace(s[:idx])
		stackTrace = strings.TrimSpace(s[idx+len("Stack:"):])
	} else {
		msg = s
	}
	if stackTrace != "" {
		stackTrace = strings.ReplaceAll(stackTrace, "\t", "  ")
	}
	return msg, stackTrace
}

func isCandidateTestResultXML(artID string) bool {
	lower := strings.ToLower(artID)
	if strings.Contains(lower, "test_result") || strings.Contains(lower, "compatibility_result") || strings.Contains(lower, "sponge_log") {
		return strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".xml.gz")
	}
	return false
}

type discoveryCacheKey struct{}

// DiscoveryCache caches work units, ancestors, artifacts, XML reasons, and discovered errors
// in memory to prevent looking up the same work unit or fetching the same artifact multiple times.
type DiscoveryCache struct {
	mu              sync.RWMutex
	errors          map[string]*DiscoveredError
	hasErrorChecked map[string]bool
	workUnits       map[string]*pb.WorkUnit
	ancestors       map[string][]*pb.WorkUnit
	artifacts       map[string][]*pb.Artifact
	xmlReasons      map[string]*XMLReason
	hasXMLChecked   map[string]bool
	rootInvocations map[string]*pb.RootInvocation
}

// NewDiscoveryCache initializes an in-memory DiscoveryCache.
func NewDiscoveryCache() *DiscoveryCache {
	return &DiscoveryCache{
		errors:          make(map[string]*DiscoveredError),
		hasErrorChecked: make(map[string]bool),
		workUnits:       make(map[string]*pb.WorkUnit),
		ancestors:       make(map[string][]*pb.WorkUnit),
		artifacts:       make(map[string][]*pb.Artifact),
		xmlReasons:      make(map[string]*XMLReason),
		hasXMLChecked:   make(map[string]bool),
		rootInvocations: make(map[string]*pb.RootInvocation),
	}
}

// WithDiscoveryCache returns a context with an in-memory DiscoveryCache attached.
func WithDiscoveryCache(ctx context.Context) context.Context {
	if cacheFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, discoveryCacheKey{}, NewDiscoveryCache())
}

func cacheFromContext(ctx context.Context) *DiscoveryCache {
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.Value(discoveryCacheKey{}).(*DiscoveryCache); ok {
		return c
	}
	return nil
}

func getWorkUnitWithCache(ctx context.Context, client pb.ResultDBClient, name string, cache *DiscoveryCache) *pb.WorkUnit {
	if cache != nil {
		cache.mu.RLock()
		if wu, ok := cache.workUnits[name]; ok {
			cache.mu.RUnlock()
			return wu
		}
		cache.mu.RUnlock()
	}
	wu, err := client.GetWorkUnit(ctx, &pb.GetWorkUnitRequest{Name: name})
	if err == nil && wu != nil {
		if cache != nil {
			cache.mu.Lock()
			cache.workUnits[name] = wu
			cache.mu.Unlock()
		}
		return wu
	}
	return nil
}

func getRootInvocationWithCache(ctx context.Context, client pb.ResultDBClient, name string, cache *DiscoveryCache) *pb.RootInvocation {
	if cache != nil {
		cache.mu.RLock()
		if rootInv, ok := cache.rootInvocations[name]; ok {
			cache.mu.RUnlock()
			return rootInv
		}
		cache.mu.RUnlock()
	}
	rootInv, err := client.GetRootInvocation(ctx, &pb.GetRootInvocationRequest{Name: name})
	if err == nil && rootInv != nil {
		if cache != nil {
			cache.mu.Lock()
			cache.rootInvocations[name] = rootInv
			cache.mu.Unlock()
		}
		return rootInv
	}
	return nil
}

// QueryAncestorWorkUnits queries the ancestor work units of targetWorkUnit.
func QueryAncestorWorkUnits(ctx context.Context, client pb.ResultDBClient, targetWorkUnit string) []*pb.WorkUnit {
	cache := cacheFromContext(ctx)
	if cache != nil {
		cache.mu.RLock()
		if anc, ok := cache.ancestors[targetWorkUnit]; ok {
			cache.mu.RUnlock()
			return anc
		}
		cache.mu.RUnlock()
	}

	rootInvID, _ := base.ExtractWorkUnitComponents(targetWorkUnit)
	if rootInvID == "" {
		return nil
	}
	rootInv := "rootInvocations/" + base.NormalizeInvocation(rootInvID)

	qRes, err := client.QueryWorkUnits(ctx, &pb.QueryWorkUnitsRequest{
		Parent: rootInv,
		Predicate: &pb.WorkUnitPredicate{
			AncestorsOf: targetWorkUnit,
		},
		PageSize: 1000,
	})
	if err != nil || qRes == nil {
		return nil
	}

	ancestors := qRes.WorkUnits
	if cache != nil {
		cache.mu.Lock()
		cache.ancestors[targetWorkUnit] = ancestors
		for _, wu := range ancestors {
			cache.workUnits[wu.Name] = wu
		}
		cache.mu.Unlock()
	}
	return ancestors
}

func queryArtifactsForParent(ctx context.Context, client pb.ResultDBClient, parent string, cache *DiscoveryCache) ([]*pb.Artifact, error) {
	if cache != nil {
		cache.mu.RLock()
		if arts, ok := cache.artifacts[parent]; ok {
			cache.mu.RUnlock()
			return arts, nil
		}
		cache.mu.RUnlock()
	}
	res, err := client.ListArtifacts(ctx, &pb.ListArtifactsRequest{
		Parent:   parent,
		PageSize: 1000,
	})
	if err != nil {
		return nil, err
	}
	if cache != nil {
		cache.mu.Lock()
		cache.artifacts[parent] = res.Artifacts
		cache.mu.Unlock()
	}
	return res.Artifacts, nil
}

func fetchAndParseXMLReasonWithCache(ctx context.Context, httpClient *http.Client, art *pb.Artifact, cache *DiscoveryCache) (*XMLReason, error) {
	key := art.Name
	if key == "" {
		key = art.ArtifactId
	}
	if key == "" {
		key = art.FetchUrl
	}
	if cache != nil {
		cache.mu.RLock()
		if cache.hasXMLChecked[key] {
			r := cache.xmlReasons[key]
			cache.mu.RUnlock()
			return r, nil
		}
		cache.mu.RUnlock()
	}

	var buf bytes.Buffer
	if err := FetchArtifactContent(ctx, httpClient, art.FetchUrl, &buf); err != nil {
		return nil, err
	}
	var r io.Reader = &buf
	data := buf.Bytes()
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b || strings.HasSuffix(art.ArtifactId, ".gz") {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			r = gz
		}
	}
	reason, err := ParseTradefedXMLReason(r)
	if cache != nil && err == nil {
		cache.mu.Lock()
		cache.hasXMLChecked[key] = true
		cache.xmlReasons[key] = reason
		cache.mu.Unlock()
	}
	return reason, err
}

// isGenericHarnessSummary checks if the summary is a generic harness-level wrapper error.
//
// TODO(b/547503348): Remove once TradeFed populates WorkUnit.SummaryMarkdown with
// specific module failure reasons instead of relying on MobileHarness wrapper summary.
func isGenericHarnessSummary(s string) bool {
	return strings.Contains(s, "ANDROID_TRADEFED_TEST_HAS_FAIL_SUBTEST")
}

// DiscoverWorkUnitError executes the 3-layer discovery hierarchy to find module / work unit errors:
// Layer 1: WorkUnit and ancestor SummaryMarkdown
// Layer 2: Tradefed test_result XML artifacts (<Reason> tags) on WorkUnit or ancestors
// Layer 3: Root Invocation SummaryMarkdown
func DiscoverWorkUnitError(ctx context.Context, rdbClient pb.ResultDBClient, httpClient *http.Client, targetWorkUnit string) (*DiscoveredError, error) {
	if targetWorkUnit == "" {
		return nil, nil
	}
	clean := base.TrimResourceURL(targetWorkUnit)
	if idx := strings.Index(clean, "/tests/"); idx != -1 {
		clean = clean[:idx]
	}

	cache := cacheFromContext(ctx)
	if cache != nil {
		cache.mu.RLock()
		if cache.hasErrorChecked[clean] {
			res := cache.errors[clean]
			cache.mu.RUnlock()
			return res, nil
		}
		cache.mu.RUnlock()
	}

	res, err := discoverWorkUnitErrorUncached(ctx, rdbClient, httpClient, clean, cache)
	if cache != nil && err == nil {
		cache.mu.Lock()
		cache.hasErrorChecked[clean] = true
		cache.errors[clean] = res
		cache.mu.Unlock()
	}
	return res, err
}

func discoverWorkUnitErrorUncached(ctx context.Context, rdbClient pb.ResultDBClient, httpClient *http.Client, clean string, cache *DiscoveryCache) (*DiscoveredError, error) {
	var fallbackSummary *DiscoveredError

	// Layer 1: Check WorkUnit SummaryMarkdown
	if strings.Contains(clean, "/workUnits/") {
		wu := getWorkUnitWithCache(ctx, rdbClient, clean, cache)
		if wu != nil && strings.TrimSpace(wu.SummaryMarkdown) != "" {
			if !isGenericHarnessSummary(wu.SummaryMarkdown) {
				return &DiscoveredError{
					Source:         fmt.Sprintf("Work Unit %s", clean),
					SourceWorkUnit: clean,
					RawSummary:     strings.TrimSpace(wu.SummaryMarkdown),
				}, nil
			}
			fallbackSummary = &DiscoveredError{
				Source:         fmt.Sprintf("Work Unit %s", clean),
				SourceWorkUnit: clean,
				RawSummary:     strings.TrimSpace(wu.SummaryMarkdown),
			}
		}
	}

	// Check Ancestors for Layer 1 SummaryMarkdown
	ancestors := QueryAncestorWorkUnits(ctx, rdbClient, clean)
	for _, anc := range ancestors {
		if strings.TrimSpace(anc.SummaryMarkdown) != "" {
			if !isGenericHarnessSummary(anc.SummaryMarkdown) {
				return &DiscoveredError{
					Source:         fmt.Sprintf("Ancestor Work Unit %s", anc.Name),
					SourceWorkUnit: anc.Name,
					RawSummary:     strings.TrimSpace(anc.SummaryMarkdown),
				}, nil
			}
			if fallbackSummary == nil {
				fallbackSummary = &DiscoveredError{
					Source:         fmt.Sprintf("Ancestor Work Unit %s", anc.Name),
					SourceWorkUnit: anc.Name,
					RawSummary:     strings.TrimSpace(anc.SummaryMarkdown),
				}
			}
		}
	}

	// Layer 2: Check Tradefed XML Artifacts on clean and ancestors.
	//
	// TODO(b/547503348): Layer 2 is a temporary fallback for TradeFed runs where module
	// errors were only recorded in test_result XML artifacts. Once TradeFed sets
	// WorkUnit.SummaryMarkdown directly, this layer can be removed.
	targetsToCheck := make([]string, 0, 1+len(ancestors))
	targetsToCheck = append(targetsToCheck, clean)
	for _, anc := range ancestors {
		targetsToCheck = append(targetsToCheck, anc.Name)
	}

	if httpClient != nil {
		for _, target := range targetsToCheck {
			arts, err := queryArtifactsForParent(ctx, rdbClient, target, cache)
			if err != nil || len(arts) == 0 {
				continue
			}
			for _, art := range arts {
				if isCandidateTestResultXML(art.ArtifactId) && art.FetchUrl != "" {
					reason, err := fetchAndParseXMLReasonWithCache(ctx, httpClient, art, cache)
					if err == nil && reason != nil {
						msg, stack := cleanAndSplitReasonMessage(reason.Message)
						return &DiscoveredError{
							Source:         fmt.Sprintf("Artifact %s", art.ArtifactId),
							SourceWorkUnit: target,
							ArtifactID:     art.ArtifactId,
							ErrorName:      reason.ErrorName,
							ErrorCode:      reason.ErrorCode,
							Message:        msg,
							StackTrace:     stack,
						}, nil
					}
				}
			}
		}
	}

	if fallbackSummary != nil {
		return fallbackSummary, nil
	}

	// Layer 3: Root Invocation SummaryMarkdown
	rootInvID, _ := base.ExtractWorkUnitComponents(clean)
	if rootInvID != "" {
		rootInvName := "rootInvocations/" + base.NormalizeInvocation(rootInvID)
		rootInv := getRootInvocationWithCache(ctx, rdbClient, rootInvName, cache)
		if rootInv != nil && strings.TrimSpace(rootInv.SummaryMarkdown) != "" {
			return &DiscoveredError{
				Source:         fmt.Sprintf("Root Invocation %s", rootInvName),
				SourceWorkUnit: rootInvName,
				RawSummary:     strings.TrimSpace(rootInv.SummaryMarkdown),
			}, nil
		}
	}

	return nil, nil
}

// PrintModuleError prints a discovered module or work unit error.
func PrintModuleError(err *DiscoveredError, indent string) {
	PrintModuleErrorForTarget(err, "", indent)
}

// PrintModuleErrorForTarget prints a discovered module or work unit error, indicating if the error originated from an ancestor work unit or root invocation.
func PrintModuleErrorForTarget(err *DiscoveredError, targetWorkUnit string, indent string) {
	if err == nil {
		return
	}

	sourceLabel := ""
	if targetWorkUnit != "" && err.SourceWorkUnit != "" && err.SourceWorkUnit != targetWorkUnit {
		if strings.Contains(err.SourceWorkUnit, "/workUnits/") {
			_, wuID := base.ExtractWorkUnitComponents(err.SourceWorkUnit)
			if wuID != "" {
				sourceLabel = fmt.Sprintf(" (from ancestor work unit %s)", wuID)
			} else {
				sourceLabel = fmt.Sprintf(" (from ancestor %s)", err.SourceWorkUnit)
			}
		} else if strings.HasPrefix(err.SourceWorkUnit, "rootInvocations/") || strings.HasPrefix(err.SourceWorkUnit, "invocations/") {
			sourceLabel = fmt.Sprintf(" (from %s)", FormatWorkUnitBreadcrumb(err.SourceWorkUnit))
		}
	}

	if err.RawSummary != "" {
		fmt.Printf("%sSummary%s:\n", indent, sourceLabel)
		for _, l := range strings.Split(strings.TrimRight(err.RawSummary, "\n"), "\n") {
			fmt.Printf("%s  %s\n", indent, l)
		}
		return
	}

	header := ""
	if err.ErrorName != "" && err.ErrorCode != "" {
		header = fmt.Sprintf("[%s|%s] ", err.ErrorName, err.ErrorCode)
	} else if err.ErrorName != "" {
		header = fmt.Sprintf("[%s] ", err.ErrorName)
	} else if err.ErrorCode != "" {
		header = fmt.Sprintf("[%s] ", err.ErrorCode)
	}

	if err.Message != "" || header != "" {
		fullMsg := header + err.Message
		lines := strings.Split(strings.TrimRight(fullMsg, "\n"), "\n")
		title := fmt.Sprintf("Module Error%s:", sourceLabel)
		if len(lines) == 1 {
			fmt.Printf("%s%s %s\n", indent, title, lines[0])
		} else {
			fmt.Printf("%s%s %s\n", indent, title, lines[0])
			for _, l := range lines[1:] {
				fmt.Printf("%s  %s\n", indent, strings.TrimSpace(l))
			}
		}
	}

	if err.StackTrace != "" {
		fmt.Printf("%sStack Trace:\n", indent)
		for _, l := range strings.Split(strings.TrimRight(err.StackTrace, "\n"), "\n") {
			fmt.Printf("%s  %s\n", indent, l)
		}
	}
}

// FormatDiscoveredErrorFirstLine returns a single-line summary of a discovered error and whether it was truncated.
func FormatDiscoveredErrorFirstLine(err *DiscoveredError, maxLen int) (string, bool) {
	if err == nil {
		return "", false
	}
	if err.RawSummary != "" {
		return TruncateFirstLine(err.RawSummary, maxLen)
	}
	header := ""
	if err.ErrorName != "" && err.ErrorCode != "" {
		header = fmt.Sprintf("[%s|%s] ", err.ErrorName, err.ErrorCode)
	} else if err.ErrorName != "" {
		header = fmt.Sprintf("[%s] ", err.ErrorName)
	} else if err.ErrorCode != "" {
		header = fmt.Sprintf("[%s] ", err.ErrorCode)
	}
	full := header + err.Message
	if full == "" {
		full = err.StackTrace
	}
	res, truncated := TruncateFirstLine(full, maxLen)
	if err.StackTrace != "" {
		truncated = true
	}
	return res, truncated
}
