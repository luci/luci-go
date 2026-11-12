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
	Source         string `json:"source,omitempty"`           // e.g. "Work Unit summary", "Artifact subprocess-test_result.xml...", "Root invocation summary"
	SourceWorkUnit string `json:"source_work_unit,omitempty"` // Resource name of the work unit or invocation where the error originated
	ArtifactID     string `json:"artifact_id,omitempty"`      // Artifact ID where the error was found (if from artifact)
	ErrorName      string `json:"error_name,omitempty"`       // e.g. "INSTRUMENTATION_NULL_METHOD"
	ErrorCode      string `json:"error_code,omitempty"`       // e.g. "530002"
	Message        string `json:"message,omitempty"`          // Failure message
	StackTrace     string `json:"stack_trace,omitempty"`      // Extracted stack trace
	RawSummary     string `json:"raw_summary,omitempty"`      // Markdown summary from WorkUnit / Invocation
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
	} else if idx := strings.Index(s, "\tat "); idx != -1 {
		msg = strings.TrimSpace(s[:idx])
		stackTrace = strings.TrimSpace(s[idx:])
	} else if idx := strings.Index(s, "\nat "); idx != -1 {
		msg = strings.TrimSpace(s[:idx])
		stackTrace = strings.TrimSpace(s[idx+1:])
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
	artifactsMore   map[string]bool
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
		artifactsMore:   make(map[string]bool),
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

// CacheResolvedInvocation stores the retrieved RootInvocation from resolved in the
// context's DiscoveryCache, if present, avoiding duplicate GetRootInvocation RPCs.
func CacheResolvedInvocation(ctx context.Context, resolved *base.ResolvedInvocation) {
	if resolved == nil || resolved.RootInvocation == nil || resolved.ID == "" {
		return
	}
	cache := cacheFromContext(ctx)
	if cache == nil {
		return
	}
	name := "rootInvocations/" + base.NormalizeInvocation(resolved.ID)
	cache.mu.Lock()
	cache.rootInvocations[name] = resolved.RootInvocation
	if resolved.RootInvocation.Name != "" {
		cache.rootInvocations[resolved.RootInvocation.Name] = resolved.RootInvocation
	}
	cache.mu.Unlock()
}

// GetCachedRootInvocation returns a previously cached RootInvocation from the
// context's DiscoveryCache, or nil if not found.
func GetCachedRootInvocation(ctx context.Context, invID string) *pb.RootInvocation {
	cache := cacheFromContext(ctx)
	if cache == nil || invID == "" {
		return nil
	}
	name := "rootInvocations/" + base.NormalizeInvocation(invID)
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	return cache.rootInvocations[name]
}

// FetchRootInvocation returns the RootInvocation for invID, using the context's
// DiscoveryCache if available to avoid duplicate RPCs.
func FetchRootInvocation(ctx context.Context, client pb.ResultDBClient, invID string) *pb.RootInvocation {
	if invID == "" {
		return nil
	}
	if rootInv := GetCachedRootInvocation(ctx, invID); rootInv != nil {
		return rootInv
	}
	if client == nil {
		return nil
	}
	name := "rootInvocations/" + base.NormalizeInvocation(invID)
	return getRootInvocationWithCache(ctx, client, name, cacheFromContext(ctx))
}

// FetchWorkUnit returns the WorkUnit for name, using the context's
// DiscoveryCache if available to avoid duplicate RPCs.
func FetchWorkUnit(ctx context.Context, client pb.ResultDBClient, name string) *pb.WorkUnit {
	if name == "" || client == nil {
		return nil
	}
	return getWorkUnitWithCache(ctx, client, name, cacheFromContext(ctx))
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

// CacheWorkUnit stores a retrieved WorkUnit in the context's DiscoveryCache,
// if present, avoiding duplicate GetWorkUnit or QueryWorkUnits RPCs.
func CacheWorkUnit(ctx context.Context, wu *pb.WorkUnit) {
	if wu == nil || wu.Name == "" {
		return
	}
	cache := cacheFromContext(ctx)
	if cache == nil {
		return
	}
	cache.mu.Lock()
	cache.workUnits[wu.Name] = wu
	cache.mu.Unlock()
}

// QueryAncestorWorkUnits queries the ancestor work units of targetWorkUnit.
func QueryAncestorWorkUnits(ctx context.Context, client pb.ResultDBClient, targetWorkUnit string) []*pb.WorkUnit {
	cache := cacheFromContext(ctx)
	var parentName string
	if cache != nil {
		cache.mu.RLock()
		if anc, ok := cache.ancestors[targetWorkUnit]; ok {
			cache.mu.RUnlock()
			return anc
		}
		if wu := cache.workUnits[targetWorkUnit]; wu != nil && wu.Parent != "" {
			parentName = wu.Parent
			if anc, ok := cache.ancestors["parent:"+parentName]; ok {
				cache.mu.RUnlock()
				return anc
			}
		}
		cache.mu.RUnlock()
	}

	rootInvID, wuID := base.ExtractWorkUnitComponents(targetWorkUnit)
	if rootInvID == "" {
		return nil
	}
	rootInv := "rootInvocations/" + base.NormalizeInvocation(rootInvID)
	if parentName == "" {
		if idx := strings.Index(wuID, ":u-"); idx > 0 {
			parentName = rootInv + "/workUnits/" + wuID[:idx]
			if cache != nil {
				cache.mu.RLock()
				if anc, ok := cache.ancestors["parent:"+parentName]; ok {
					cache.mu.RUnlock()
					return anc
				}
				cache.mu.RUnlock()
			}
		}
	}

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
		if parentName != "" {
			cache.ancestors["parent:"+parentName] = ancestors
		}
		if len(ancestors) > 0 && ancestors[0].GetName() != "" {
			cache.ancestors["parent:"+ancestors[0].GetName()] = ancestors
		}
		for _, wu := range ancestors {
			cache.workUnits[wu.Name] = wu
		}
		cache.mu.Unlock()
	}
	return ancestors
}

// QueryArtifactsForParent lists up to 1000 artifacts for parent, using the
// context's DiscoveryCache if available to avoid duplicate ListArtifacts RPCs.
// It returns the artifacts, whether additional pages exist (NextPageToken != ""),
// and any RPC error.
func QueryArtifactsForParent(ctx context.Context, client pb.ResultDBClient, parent string) ([]*pb.Artifact, bool, error) {
	return queryArtifactsForParentWithMore(ctx, client, parent, cacheFromContext(ctx))
}

func queryArtifactsForParent(ctx context.Context, client pb.ResultDBClient, parent string, cache *DiscoveryCache) ([]*pb.Artifact, error) {
	arts, _, err := queryArtifactsForParentWithMore(ctx, client, parent, cache)
	return arts, err
}

func queryArtifactsForParentWithMore(ctx context.Context, client pb.ResultDBClient, parent string, cache *DiscoveryCache) ([]*pb.Artifact, bool, error) {
	if cache != nil {
		cache.mu.RLock()
		if arts, ok := cache.artifacts[parent]; ok {
			more := cache.artifactsMore[parent]
			cache.mu.RUnlock()
			return arts, more, nil
		}
		cache.mu.RUnlock()
	}
	res, err := client.ListArtifacts(ctx, &pb.ListArtifactsRequest{
		Parent:   parent,
		PageSize: 1000,
	})
	if err != nil {
		return nil, false, err
	}
	more := res.NextPageToken != ""
	if cache != nil {
		cache.mu.Lock()
		cache.artifacts[parent] = res.Artifacts
		cache.artifactsMore[parent] = more
		cache.mu.Unlock()
	}
	return res.Artifacts, more, nil
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

// IsGenericHarnessSummary reports whether s is a generic MobileHarness wrapper
// error summary (ANDROID_TRADEFED_TEST_HAS_FAIL_SUBTEST) indicating only that
// sub-tests failed without a specific module/harness root cause.
//
// TODO(b/547503348): Remove once TradeFed populates WorkUnit.SummaryMarkdown with
// specific module failure reasons instead of relying on MobileHarness wrapper summary.
func IsGenericHarnessSummary(s string) bool {
	return strings.Contains(s, "ANDROID_TRADEFED_TEST_HAS_FAIL_SUBTEST")
}

func isFallbackWorkUnitSummary(ctx context.Context, rdbClient pb.ResultDBClient, wuName, summary string, cache *DiscoveryCache) bool {
	if IsGenericHarnessSummary(summary) {
		return true
	}
	if !strings.HasSuffix(wuName, "/workUnits/root") {
		return false
	}
	rootInvID, _ := base.ExtractWorkUnitComponents(wuName)
	if rootInvID == "" {
		return false
	}
	rootInvName := "rootInvocations/" + base.NormalizeInvocation(rootInvID)
	rootInv := getRootInvocationWithCache(ctx, rdbClient, rootInvName, cache)
	return rootInv != nil && IsRedundantExternalSummary(summary, rootInv)
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
			if !isFallbackWorkUnitSummary(ctx, rdbClient, clean, wu.SummaryMarkdown, cache) {
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
			if !isFallbackWorkUnitSummary(ctx, rdbClient, anc.Name, anc.SummaryMarkdown, cache) {
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

// maxTracePreviewLines is the maximum number of stack trace lines rendered
// inline when displaying an interesting failure artifact preview.
const maxTracePreviewLines = 40

// InterestingArtifact represents a notable artifact on a failed or skipped
// work unit (such as a full failure summary or stack trace) to surface in CLI output.
type InterestingArtifact struct {
	ID              string
	Title           string // section heading for inline preview (e.g. "Invocation Failure Trace")
	Label           string // description for the fetch hint (e.g. "failure summary", "failure trace")
	Preview         string // inline text content (when <= maxInlineArtifactBytes)
	MaxPreviewLines int    // max lines to render inline (0 when content is merged into FullSummaryText)
}

// WorkUnitFailure holds failure details and interesting artifacts discovered
// on a work unit.
type WorkUnitFailure struct {
	WorkUnitID      string
	Kind            string
	State           pb.WorkUnit_State
	SummaryMarkdown string
	FullSummaryText string
	Artifacts       []InterestingArtifact
}

// DiscoverSingleWorkUnitFailure checks wu for interesting failure or skip
// artifacts (ordered by interestingArtifactRules), fetching bounded inline
// previews in parallel when httpClient is provided.
func DiscoverSingleWorkUnitFailure(ctx context.Context, rdbClient pb.ResultDBClient, httpClient *http.Client, wu *pb.WorkUnit) *WorkUnitFailure {
	if rdbClient == nil || wu.GetName() == "" || !isFailedOrSkippedWorkUnit(wu) {
		return nil
	}
	wuID := wu.GetWorkUnitId()
	if wuID == "" {
		_, wuID = base.ExtractWorkUnitComponents(wu.GetName())
	}
	arts, _ := queryArtifactsForParent(ctx, rdbClient, wu.GetName(), cacheFromContext(ctx))
	f := &WorkUnitFailure{
		WorkUnitID:      wuID,
		Kind:            wu.GetKind(),
		State:           wu.GetState(),
		SummaryMarkdown: strings.TrimSpace(wu.GetSummaryMarkdown()),
	}
	type matchedArtifact struct {
		rule failureArtifactRule
		art  *pb.Artifact
	}
	var matched []matchedArtifact
	for _, rule := range interestingArtifactRules {
		for _, art := range arts {
			if strings.HasPrefix(art.GetArtifactId(), rule.prefix) {
				matched = append(matched, matchedArtifact{rule: rule, art: art})
			}
		}
	}
	if len(matched) == 0 {
		return f
	}
	texts := make([]string, len(matched))
	var wg sync.WaitGroup
	for i, m := range matched {
		wg.Add(1)
		go func(idx int, art *pb.Artifact) {
			defer wg.Done()
			texts[idx] = fetchTextArtifact(ctx, httpClient, art)
		}(i, m.art)
	}
	wg.Wait()

	var summaries []string
	f.Artifacts = make([]InterestingArtifact, 0, len(matched))
	for i, m := range matched {
		txt := texts[i]
		if m.rule.isSummary && txt != "" {
			summaries = append(summaries, txt)
		}
		f.Artifacts = append(f.Artifacts, InterestingArtifact{
			ID:              m.art.GetArtifactId(),
			Title:           m.rule.title,
			Label:           m.rule.label,
			Preview:         txt,
			MaxPreviewLines: m.rule.maxPreviewLines,
		})
	}
	f.FullSummaryText = strings.Join(summaries, "\n\n")
	return f
}

// DiscoverFirstLevelWorkUnitFailures inspects direct (first-level) child work
// units of the root work unit for failure summaries and interesting artifacts.
func DiscoverFirstLevelWorkUnitFailures(ctx context.Context, rdbClient pb.ResultDBClient, httpClient *http.Client, invID string, rootWU *pb.WorkUnit) []*WorkUnitFailure {
	if rdbClient == nil || invID == "" {
		return nil
	}
	normInv := base.NormalizeInvocation(invID)
	cache := cacheFromContext(ctx)
	if rootWU == nil {
		rootWU = getWorkUnitWithCache(ctx, rdbClient, base.FormatWorkUnitResourceName(normInv, "root"), cache)
	}
	if rootWU.GetState() == pb.WorkUnit_SUCCEEDED {
		return nil
	}
	childNames := rootWU.GetChildWorkUnits()
	if len(childNames) == 0 {
		return nil
	}
	batchRes, err := rdbClient.BatchGetWorkUnits(ctx, &pb.BatchGetWorkUnitsRequest{
		Parent: "rootInvocations/" + normInv,
		Names:  childNames,
	})
	children := batchRes.GetWorkUnits()
	if err == nil && len(children) > 0 && cache != nil {
		cache.mu.Lock()
		for _, wu := range children {
			if wu.GetName() != "" {
				cache.workUnits[wu.GetName()] = wu
			}
		}
		cache.mu.Unlock()
	}
	if err != nil || len(children) == 0 {
		for _, name := range childNames {
			if wu := getWorkUnitWithCache(ctx, rdbClient, name, cache); wu != nil {
				children = append(children, wu)
			}
		}
	}
	discovered := make([]*WorkUnitFailure, len(children))
	var wg sync.WaitGroup
	for i, child := range children {
		if !isFailedOrSkippedWorkUnit(child) {
			continue
		}
		wg.Add(1)
		go func(idx int, wu *pb.WorkUnit) {
			defer wg.Done()
			discovered[idx] = DiscoverSingleWorkUnitFailure(ctx, rdbClient, httpClient, wu)
		}(i, child)
	}
	wg.Wait()

	var results []*WorkUnitFailure
	for i, cf := range discovered {
		if cf == nil {
			continue
		}
		hasSummary := cf.SummaryMarkdown != "" && !IsGenericHarnessSummary(cf.SummaryMarkdown)
		if children[i].GetState() == pb.WorkUnit_FAILED || hasSummary || len(cf.Artifacts) > 0 {
			results = append(results, cf)
		}
	}
	return results
}

// FprintWorkUnitFailureArtifacts writes any inline previews and copy-pasteable
// 'luci artifact get' commands for the interesting artifacts in f to out.
func FprintWorkUnitFailureArtifacts(out io.Writer, invID string, f *WorkUnitFailure, indent string) {
	if out == nil || f == nil {
		return
	}
	normInv := base.NormalizeInvocation(invID)
	for _, art := range f.Artifacts {
		if art.Preview != "" && art.MaxPreviewLines > 0 {
			title := art.Title
			if title == "" {
				title = "Artifact Preview"
			}
			fmt.Fprintf(out, "%s%s (%s):\n", indent, title, art.ID)
			lines := strings.Split(strings.ReplaceAll(art.Preview, "\r\n", "\n"), "\n")
			shown := lines
			if len(lines) > art.MaxPreviewLines {
				shown = lines[:art.MaxPreviewLines]
			}
			for _, line := range shown {
				fmt.Fprintf(out, "%s  %s\n", indent, line)
			}
			if extra := len(lines) - len(shown); extra > 0 {
				fmt.Fprintf(out, "%s  ... (%d more lines)\n", indent, extra)
			}
		}
		label := art.Label
		if label == "" {
			label = "artifact"
		}
		fmt.Fprintf(out, "%sTo fetch full %s (%s):\n", indent, label, art.ID)
		fmt.Fprintf(out, "%s  luci artifact get -invocationid=%s -workunitid=%s -artifactid=%s\n",
			indent, base.ShellQuoteIfNeeded(normInv), base.ShellQuoteIfNeeded(f.WorkUnitID), base.ShellQuoteIfNeeded(art.ID))
	}
}

// FprintFirstLevelWorkUnitFailures writes failed first-level child work units,
// their summaries, and their interesting artifact previews and fetch commands.
func FprintFirstLevelWorkUnitFailures(out io.Writer, invID string, failures []*WorkUnitFailure, indent string) {
	for _, f := range failures {
		if out == nil || f == nil {
			continue
		}
		var meta []string
		if f.Kind != "" {
			meta = append(meta, "kind: "+f.Kind)
		}
		if f.State != pb.WorkUnit_STATE_UNSPECIFIED {
			meta = append(meta, "state: "+f.State.String())
		}
		suffix := ""
		if len(meta) > 0 {
			suffix = " (" + strings.Join(meta, ", ") + ")"
		}
		heading := "Failed Child Work Unit"
		switch f.State {
		case pb.WorkUnit_SKIPPED:
			heading = "Skipped Child Work Unit"
		case pb.WorkUnit_CANCELLED:
			heading = "Cancelled Child Work Unit"
		}
		fmt.Fprintf(out, "%s%s: %s%s\n", indent, heading, f.WorkUnitID, suffix)
		summary := f.FullSummaryText
		if summary == "" {
			summary = f.SummaryMarkdown
		}
		if summary != "" {
			fmt.Fprintf(out, "%s  Summary:\n", indent)
			for _, line := range strings.Split(strings.ReplaceAll(summary, "\r\n", "\n"), "\n") {
				fmt.Fprintf(out, "%s    %s\n", indent, line)
			}
		}
		FprintWorkUnitFailureArtifacts(out, invID, f, indent+"  ")
	}
}

type failureArtifactRule struct {
	prefix          string
	title           string
	label           string
	isSummary       bool
	maxPreviewLines int
}

// interestingArtifactRules is the ordered list of artifact ID prefixes to
// surface when inspecting a failed or skipped work unit.
//
// Other test harnesses/projects can be supported by adding their failure or
// diagnostic artifact prefixes in display priority order.
var interestingArtifactRules = []failureArtifactRule{
	// TradeFed-specific artifacts uploaded by GoogleTradefedResultReporter
	// when a command work unit fails or is skipped (FullSummaryMarkdown is
	// uploaded as an artifact because WorkUnit.SummaryMarkdown is capped at 4 KB).
	{
		prefix:    "invocationFailed_summary_markdown",
		label:     "failure summary",
		isSummary: true,
	},
	{
		prefix:    "invocationSkipped_summary_markdown",
		label:     "skipped summary",
		isSummary: true,
	},
	{
		prefix:          "invocationFailed_trace",
		title:           "Invocation Failure Trace",
		label:           "failure trace",
		maxPreviewLines: maxTracePreviewLines,
	},
}

func isFailedOrSkippedWorkUnit(wu *pb.WorkUnit) bool {
	switch wu.GetState() {
	case pb.WorkUnit_SUCCEEDED:
		return false
	case pb.WorkUnit_FAILED, pb.WorkUnit_SKIPPED, pb.WorkUnit_CANCELLED:
		return true
	default:
		return strings.TrimSpace(wu.GetSummaryMarkdown()) != ""
	}
}

func fetchTextArtifact(ctx context.Context, httpClient *http.Client, art *pb.Artifact) string {
	if httpClient == nil || art.GetFetchUrl() == "" || art.GetSizeBytes() < 0 || art.GetSizeBytes() > maxInlineArtifactBytes {
		return ""
	}
	var buf bytes.Buffer
	if err := FetchArtifactContent(ctx, httpClient, art.GetFetchUrl(), &buf); err != nil || buf.Len() > maxInlineArtifactBytes {
		return ""
	}
	return strings.TrimSpace(buf.String())
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
