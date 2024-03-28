// Copyright 2020 The LUCI Authors.
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

package gerritfake

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/cv/internal/gerrit"
)

// Fake simulates Gerrit for CV tests.
type Fake struct {
	// m protects all other members below.
	m sync.Mutex

	// cs is a set of changes, indexed by (host, change number).
	// See key() function.
	cs map[string]*Change

	// parentsOf maps a change's patchset (host, change number, patchset)
	// to one or more Git parents; each parent is another change's patchset.
	//
	// parentsOf[X] can be read as "changes on which X depends non-transitively".
	//
	// parentsOf is essentially the DAG (directed acyclic graph) that Git stores.
	parentsOf map[string][]string
	// childrenOf is a reverse of parentsOf.
	//
	// childrenOf[X] can be read as "changes which depend on X non-transitively".
	childrenOf map[string][]string

	// linkedAccounts is set of linked email addresses for a given email.
	linkedAccounts map[string][]*gerritpb.EmailInfo

	// requests are all incoming requests that this Fake has received.
	requests   []proto.Message
	requestsMu sync.RWMutex
}

// MakeClient implemnents gerrit.Factory.
func (f *Fake) MakeClient(ctx context.Context, gerritHost, luciProject string) (gerrit.Client, error) {
	if strings.ContainsRune(luciProject, '.') {
		// Quickly catch common mistake.
		panic(fmt.Errorf("wrong gerritHost or luciProject: %q %q", gerritHost, luciProject))
	}
	return &Client{f: f, luciProject: luciProject, host: gerritHost}, nil
}

// MakeMirrorIterator implemnents gerrit.Factory.
func (f *Fake) MakeMirrorIterator(ctx context.Context) *gerrit.MirrorIterator {
	return &gerrit.MirrorIterator{""}
}

// Requests returns a shallow copy of all incoming requests this fake has
// received.
func (f *Fake) Requests() []proto.Message {
	f.requestsMu.RLock()
	defer f.requestsMu.RUnlock()
	cpy := make([]proto.Message, len(f.requests))
	copy(cpy, f.requests)
	return cpy
}

func (f *Fake) recordRequest(req proto.Message) {
	f.requestsMu.Lock()
	defer f.requestsMu.Unlock()
	f.requests = append(f.requests, proto.Clone(req))
}

// Change = change details + ACLs.
type Change struct {
	Host string
	Info *gerritpb.ChangeInfo
	ACLs AccessCheck
}

// Copy deep-copies a Change.
// NOTE: ACLs, which is a reference to a func, isn't deep-copied.
func (c *Change) Copy() *Change {
	r := &Change{
		Host: c.Host,
		Info: proto.Clone(c.Info).(*gerritpb.ChangeInfo),
		ACLs: c.ACLs,
	}
	return r
}

type AccessCheck func(op Operation, luciProject string) *status.Status

type Operation int

const (
	// OpRead gates Fetch CL metadata, files, related CLs.
	OpRead Operation = iota
	// OpReview gates posting comments and votes on one's own behalf.
	//
	// NOTE: The actual Gerrit service has per-label ACLs for voting, but CV
	// doesn't vote on its own.
	OpReview
	// OpAlterVotesOfOthers gates altering votes of behalf of others.
	OpAlterVotesOfOthers
	// OpSubmit gates submitting.
	OpSubmit
)

///////////////////////////////////////////////////////////////////////////////
// Antiboilerplate functions to reduce verbosity in tests.

// WithCLs returns Fake with several changes.
func WithCLs(cs ...*Change) *Fake {
	f := &Fake{
		cs: make(map[string]*Change, len(cs)),
	}
	for _, c := range cs {
		cpy := &Change{
			Host: c.Host,
			ACLs: c.ACLs,
			Info: &gerritpb.ChangeInfo{},
		}
		proto.Merge(cpy.Info, c.Info)
		f.cs[c.key()] = cpy
	}
	return f
}

// WithCIs returns a Fake with one change per passed ChangeInfo sharing the same
// host and ACLs.
func WithCIs(host string, acls AccessCheck, cis ...*gerritpb.ChangeInfo) *Fake {
	f := &Fake{}
	f.cs = make(map[string]*Change, len(cis))
	for _, ci := range cis {
		c := &Change{
			Host: host,
			ACLs: acls,
			Info: &gerritpb.ChangeInfo{},
		}
		proto.Merge(c.Info, ci)
		f.cs[c.key()] = c
	}
	return f
}

// AddFrom adds all changes from another fake to the this fake and returns this
// fake.
//
// Changes are added by reference. Primarily useful to construct Fake with CLs
// on several hosts, e.g.:
//
//	fake := WithCIs(hostA, aclA, ciA1, ciA2).AddFrom(hostB, aclB, ciB1)
func (f *Fake) AddFrom(other *Fake) *Fake {
	f.m.Lock()
	defer f.m.Unlock()
	other.m.Lock()
	defer other.m.Unlock()

	if f.cs == nil {
		f.cs = make(map[string]*Change, len(other.cs))
	}
	for k, c := range other.cs {
		if f.cs[k] != nil {
			panic(fmt.Errorf("change %s defined in both fakes", k))
		}
		f.cs[k] = c
	}

	if f.childrenOf == nil {
		f.childrenOf = make(map[string][]string, len(other.childrenOf))
	}
	for k, vs := range other.childrenOf {
		f.childrenOf[k] = append(f.childrenOf[k], vs...)
	}

	if f.parentsOf == nil {
		f.parentsOf = make(map[string][]string, len(other.parentsOf))
	}
	for k, vs := range other.parentsOf {
		f.parentsOf[k] = append(f.parentsOf[k], vs...)
	}
	return f
}

type CIModifier func(ci *gerritpb.ChangeInfo)

// CI creates a new ChangeInfo with 1 patchset with status NEW and without any
// votes.
func CI(change int, mods ...CIModifier) *gerritpb.ChangeInfo {
	rev := Rev(change, 1)
	ci := &gerritpb.ChangeInfo{
		Number:  int64(change),
		Project: "infra/infra",
		Ref:     "refs/heads/main",
		Status:  gerritpb.ChangeStatus_NEW,
		Owner:   U("owner-99"),

		Created: timestamppb.New(testclock.TestRecentTimeUTC.Add(1 * time.Hour)),
		Updated: timestamppb.New(testclock.TestRecentTimeUTC.Add(2 * time.Hour)),

		CurrentRevision: rev,
		Revisions: map[string]*gerritpb.RevisionInfo{
			rev: RevInfo(1),
		},
	}
	for _, m := range mods {
		m(ci)
	}
	return ci
}

func RevInfo(ps int) *gerritpb.RevisionInfo {
	return &gerritpb.RevisionInfo{
		Number:  int32(ps),
		Kind:    gerritpb.RevisionInfo_REWORK,
		Created: timestamppb.New(testclock.TestRecentTimeUTC.Add(1 * time.Hour).Add(time.Duration(ps) * time.Minute)),
		Files: map[string]*gerritpb.FileInfo{
			fmt.Sprintf("ps%03d/c.cpp", ps): {Status: gerritpb.FileInfo_W},
			"shared/s.py":                   {Status: gerritpb.FileInfo_W},
		},
		Commit: &gerritpb.CommitInfo{
			Id: "", // Id isn't set by Gerrit. It's set as a key in the revisions map.
			Parents: []*gerritpb.CommitInfo_Parent{
				{Id: "fake_parent_commit"},
			},
			Message: "Commit.\n\nDescription.",
		},
	}
}

// Rev generates revision in the form "rev-000006-013" where 6 and 13 are change and
// patchset numbers, respectively.
func Rev(ch, ps int) string {
	return fmt.Sprintf("rev-%06d-%03d", ch, ps)
}

// RelatedChange returns ChangeAndCommit for the GetRelatedChangesResponse.
//
// Parents can be specified in several ways:
//   - gerritpb.CommitInfo_Parent
//   - gerritpb.CommitInfo
//   - "<change>_<patchset>", e.g. "123_4"
//   - "<revision>" (without underscores).
func RelatedChange(change, ps, curPs int, parents ...any) *gerritpb.GetRelatedChangesResponse_ChangeAndCommit {
	prs := make([]*gerritpb.CommitInfo_Parent, len(parents))
	for i, pi := range parents {
		switch v := pi.(type) {
		case *gerritpb.CommitInfo_Parent:
			prs[i] = v
		case *gerritpb.CommitInfo:
			prs[i] = &gerritpb.CommitInfo_Parent{Id: v.GetId()}
		case string:
			if j := strings.IndexRune(v, '_'); j != -1 {
				prs[i] = &gerritpb.CommitInfo_Parent{Id: Rev(atoi(v[:j]), atoi(v[j+1:]))}
			} else {
				prs[i] = &gerritpb.CommitInfo_Parent{Id: v}
			}
		default:
			panic(fmt.Errorf("unsupported type %T as commit parent #%d", pi, i))
		}
	}
	return &gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
		CurrentPatchset: int64(curPs),
		Number:          int64(change),
		Patchset:        int64(ps),
		Commit: &gerritpb.CommitInfo{
			Id:      Rev(change, ps),
			Parents: prs,
		},
	}
}

// ACLRestricted grants full access to specified projects only.
func ACLRestricted(luciProjects ...string) AccessCheck {
	ps := stringset.NewFromSlice(luciProjects...)
	return func(_ Operation, luciProject string) *status.Status {
		if ps.Has(luciProject) {
			return status.New(codes.OK, "")
		}
		return status.New(codes.NotFound, "")
	}
}

// ACLPublic grants what every registered user can do on public projects.
func ACLPublic() AccessCheck {
	return func(op Operation, _ string) *status.Status {
		switch op {
		case OpRead, OpReview:
			return status.New(codes.OK, "")
		default:
			return status.New(codes.PermissionDenied, "can read, can't modify")
		}
	}
}

// ACLReadOnly grants read-only access to the given projects.
func ACLReadOnly(luciProjects ...string) AccessCheck {
	ps := stringset.NewFromSlice(luciProjects...)
	return func(op Operation, p string) *status.Status {
		switch {
		case !ps.Has(p):
			return status.New(codes.NotFound, "")
		case op == OpRead:
			return status.New(codes.OK, "")
		default:
			return status.New(codes.PermissionDenied, "can read, can't modify")
		}
	}
}

// ACLGrant grants a permission to given projects.
func ACLGrant(op Operation, code codes.Code, luciProjects ...string) AccessCheck {
	ps := stringset.NewFromSlice(luciProjects...)
	return func(o Operation, p string) *status.Status {
		if ps.Has(p) && o == op {
			return status.New(codes.OK, "")
		}
		return status.New(code, "")
	}
}

// Or returns the "less restrictive" status of the 2+ AccessChecks.
//
// {OK, FAILED_PRECONDITION} <= PERMISSION_DENIED <= NOT_FOUND.
// Doesn't work well with other statuses.
func (a AccessCheck) Or(bs ...AccessCheck) AccessCheck {
	return func(op Operation, luciProject string) *status.Status {
		ret := a(op, luciProject)
		switch ret.Code() {
		case codes.OK, codes.FailedPrecondition:
			return ret
		}
		for _, b := range bs {
			s := b(op, luciProject)
			switch s.Code() {
			case codes.OK, codes.FailedPrecondition:
				return s
			case codes.PermissionDenied:
				ret = s
			}
		}
		return ret
	}
}

///////////////////////////////////////////////////////////////////////////////
// CI Modifiers

// PS ensures ChangeInfo's CurrentRevision corresponds to given patchset,
// and deletes all revisions with bigger patchsets.
func PS(ps int) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		var toDelete []string
		found := false
		for rev, ri := range ci.GetRevisions() {
			switch latest := int(ri.GetNumber()); {
			case latest == ps:
				ci.CurrentRevision = rev
				found = true
			case latest > ps:
				toDelete = append(toDelete, rev)
			}
		}
		for _, rev := range toDelete {
			delete(ci.GetRevisions(), rev)
		}
		if !found {
			rev := Rev(int(ci.GetNumber()), ps)
			ci.CurrentRevision = rev
			ci.GetRevisions()[rev] = RevInfo(int(ps))
		}
	}
}

// PSWithUploader does the same as PS, but attaches a user and creation
// timestamp to the patchset.
func PSWithUploader(ps int, username string, creationTime time.Time) CIModifier {
	barePS := PS(ps)
	return func(ci *gerritpb.ChangeInfo) {
		barePS(ci)
		for _, ri := range ci.GetRevisions() {
			if int(ri.GetNumber()) == ps {
				ri.Uploader = U(username)
				ri.Created = timestamppb.New(creationTime)
			}
		}
	}
}

// AllRevs ensures ChangeInfo has a RevisionInfo per each revision
// corresponding to patchsets 1..current.
func AllRevs() CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		max := int(ci.GetRevisions()[ci.GetCurrentRevision()].GetNumber())
		found := make([]bool, max)
		for _, ri := range ci.GetRevisions() {
			found[ri.GetNumber()-1] = true
		}
		for i, f := range found {
			if !f {
				ps := i + 1
				ci.GetRevisions()[Rev(int(ci.GetNumber()), ps)] = RevInfo(ps)
			}
		}
	}
}

// Files sets ChangeInfo's current revision to contain given files.
func Files(fs ...string) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		ri := ci.GetRevisions()[ci.GetCurrentRevision()]
		m := make(map[string]*gerritpb.FileInfo, len(fs))
		for _, f := range fs {
			// CV doesn't actually care what status is.
			m[f] = &gerritpb.FileInfo{}
		}
		ri.Files = m
	}
}

// Desc sets commit message, aka CL description, for ChangeInfo's current
// revision.
func Desc(cldescription string) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		ri := ci.GetRevisions()[ci.GetCurrentRevision()]
		ri.GetCommit().Message = cldescription
	}
}

// Owner sets .Owner to the given username.
//
// See U() for format.
func Owner(username string) CIModifier {
	a := U(username) // fail fast if wrong format
	return func(ci *gerritpb.ChangeInfo) {
		ci.Owner = a
	}
}

// Updated sets .Updated to the given time.
func Updated(t time.Time) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		ci.Updated = timestamppb.New(t)
	}
}

// Ref sets .Ref to the given ref.
func Ref(ref string) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		if !strings.HasPrefix(ref, "refs/") {
			panic(fmt.Errorf("ref must start with 'refs/', but %q given", ref))
		}
		ci.Ref = ref
	}
}

// Project sets .Project to the given Gerrit project.
func Project(p string) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		ci.Project = p
	}
}

// Status sets .Status to the given status.
// Either a string or value of gerritpb.ChangeStatus.
func Status(s any) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		switch v := s.(type) {
		case gerritpb.ChangeStatus:
			ci.Status = v
			return
		case string:
			if i, exists := gerritpb.ChangeStatus_value[v]; exists {
				ci.Status = gerritpb.ChangeStatus(i)
				return
			}
		}
		panic(fmt.Errorf("unrecognized status %v", s))
	}
}

// Messages sets .Messages to the given messages.
func Messages(msgs ...*gerritpb.ChangeMessageInfo) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		ci.Messages = msgs
	}
}

// Vote sets a label to the given value by the given user(s) on the latest
// patchset.
func Vote(label string, value int, timeAndUser ...any) CIModifier {
	var who *gerritpb.AccountInfo
	var when time.Time
	switch {
	case len(timeAndUser) == 0:
		// Larger than default rev creation time even with lots of patchsets.
		when = testclock.TestRecentTimeUTC.Add(10 * time.Hour)
		who = U("user-1")
	case len(timeAndUser) != 2:
		panic(fmt.Errorf("incorrect usage, must have 2 params, not %d", len(timeAndUser)))
	default:
		var ok bool
		if when, ok = timeAndUser[0].(time.Time); !ok {
			panic(fmt.Errorf("expected time.Time, got %T", timeAndUser[0]))
		}

		switch v := timeAndUser[1].(type) {
		case *gerritpb.AccountInfo:
			who = v
		case string:
			who = U(v)
		default:
			panic(fmt.Errorf("expected *gerritpb.AccountInfo or string, got %T", v))
		}
	}

	ai := &gerritpb.ApprovalInfo{
		User:  who,
		Date:  timestamppb.New(when),
		Value: int32(value),
	}
	return func(ci *gerritpb.ChangeInfo) {
		if ci.GetLabels() == nil {
			ci.Labels = map[string]*gerritpb.LabelInfo{}
		}
		switch li, ok := ci.GetLabels()[label]; {
		case !ok:
			ci.GetLabels()[label] = &gerritpb.LabelInfo{
				All: []*gerritpb.ApprovalInfo{ai},
			}
		case ok:
			for i, existing := range li.GetAll() {
				if existing.GetUser().GetAccountId() == ai.GetUser().GetAccountId() {
					li.All[i] = ai
					return
				}
			}
			li.All = append(li.GetAll(), ai)
		}
	}
}

// CQ is a shorthand for Vote("Commit-Queue", ...).
func CQ(value int, timeAndUser ...any) CIModifier {
	return Vote("Commit-Queue", value, timeAndUser...)
}

// Approve sets Submittable to true.
func Approve() CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		ci.Submittable = true
	}
}

// Disapprove sets Submittable to false.
func Disapprove() CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		ci.Submittable = false
	}
}

// Reviewer sets the reviewers of the CL.
func Reviewer(rs ...*gerritpb.AccountInfo) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		if ci.Reviewers == nil {
			ci.Reviewers = &gerritpb.ReviewerStatusMap{}
		}
		ci.Reviewers.Reviewers = rs
	}
}

var usernameToAccountIDRegexp = regexp.MustCompile(`^.+[-.\alpha](\d+)$`)

// U returns a Gerrit User for `username`@example.com as gerritpb.AccountInfo.
//
// AccountID is either 1 or taken from the ending digits of a username.
func U(username string) *gerritpb.AccountInfo {
	accountID := int64(1)
	if subs := usernameToAccountIDRegexp.FindSubmatch([]byte(username)); len(subs) > 0 {
		i, err := strconv.ParseInt(string(subs[1]), 10, 64)
		if err != nil {
			panic(err)
		}
		accountID = i
	}
	email := username + "@example.com"
	return &gerritpb.AccountInfo{
		Email:     email,
		AccountId: accountID,
	}
}

// MetaRevID sets .MetaRevID for the given change.
func MetaRevID(metaRevID string) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		ci.MetaRevId = metaRevID
	}
}

// ParentCommits sets the parent commits for the current revision.
func ParentCommits(parents []string) CIModifier {
	return func(ci *gerritpb.ChangeInfo) {
		if ci.GetCurrentRevision() == "" {
			panic("missing current revision")
		}
		revInfo, ok := ci.GetRevisions()[ci.GetCurrentRevision()]
		if !ok {
			panic("missing revision info for current revision")
		}

		revInfo.GetCommit().Parents = make([]*gerritpb.CommitInfo_Parent, len(parents))
		for i, parent := range parents {
			revInfo.GetCommit().Parents[i] = &gerritpb.CommitInfo_Parent{
				Id: parent,
			}
		}
	}
}

///////////////////////////////////////////////////////////////////////////////
// Getters / Mutators

// Has returns if given change exists.
func (f *Fake) Has(host string, change int) bool {
	f.m.Lock()
	defer f.m.Unlock()
	_, ok := f.cs[key(host, change)]
	return ok
}

// GetChange returns a copy of a Change that must exist. Panics otherwise.
func (f *Fake) GetChange(host string, change int) *Change {
	f.m.Lock()
	defer f.m.Unlock()
	c, ok := f.cs[key(host, change)]
	if !ok {
		panic(fmt.Errorf("CL %s/%d not found", host, change))
	}
	return c.Copy()
}

// CreateChange adds a change that must not yet exist.
func (f *Fake) CreateChange(c *Change) {
	f.m.Lock()
	defer f.m.Unlock()
	k := key(c.Host, int(c.Info.GetNumber()))
	if f.cs == nil {
		f.cs = map[string]*Change{k: c}
		return
	}
	if _, ok := f.cs[k]; ok {
		panic(fmt.Errorf("CL %s already exists", k))
	}
	f.cs[k] = c.Copy()
}

// MutateChange modifies a change while holding a lock blocking concurrent RPCs.
// Change must exist. Panics otherwise.
func (f *Fake) MutateChange(host string, change int, mut func(c *Change)) {
	k := key(host, change)

	f.m.Lock()
	defer f.m.Unlock()
	c, ok := f.cs[k]
	if !ok {
		panic(fmt.Errorf("CL %s/%d not found", host, change))
	}
	mut(c)
	// Make a copy, to avoid accidental mutation at call sites.
	f.cs[k] = c.Copy()
}

// DeleteChange deletes a change that must exist. Panics otherwise.
func (f *Fake) DeleteChange(host string, change int) {
	k := key(host, change)
	f.m.Lock()
	defer f.m.Unlock()
	if _, ok := f.cs[k]; !ok {
		panic(fmt.Errorf("CL %s/%d not found", host, change))
	}
	delete(f.cs, k)
}

// SetDependsOn establishes Git relationship between a child CL and 1 or more
// parents, which are considered dependencies of the child CL.
//
// Child and each parent can be specified as either:
//   - Change or ChangeInfo, in which case their current patchset is used,
//   - <change>_<patchset>, e.g. "10_3".
func (f *Fake) SetDependsOn(host string, child any, parents ...any) {
	f.m.Lock()
	defer f.m.Unlock()
	if f.parentsOf == nil {
		f.parentsOf = make(map[string][]string, 1)
	}
	if f.childrenOf == nil {
		f.childrenOf = make(map[string][]string, len(parents))
	}

	ch, ps := parseChangePatchset(child)
	ckey := psKey(host, ch, ps)
	if _, _, _, err := f.resolvePSKeyLocked(ckey); err != nil {
		panic(err)
	}
	for _, p := range parents {
		ch, ps = parseChangePatchset(p)
		pkey := psKey(host, ch, ps)
		if pkey == ckey {
			panic(fmt.Errorf("same child %q and parent %q", ckey, pkey))
		}
		if _, _, _, err := f.resolvePSKeyLocked(pkey); err != nil {
			panic(err)
		}
		f.parentsOf[ckey] = append(f.parentsOf[ckey], pkey)
		f.childrenOf[pkey] = append(f.childrenOf[pkey], ckey)
	}
}

// AddLinkedAccountMapping maps each of the email to the set of email addresses.
func (f *Fake) AddLinkedAccountMapping(emails []*gerritpb.EmailInfo) {
	f.m.Lock()
	defer f.m.Unlock()

	if f.linkedAccounts == nil {
		f.linkedAccounts = make(map[string][]*gerritpb.EmailInfo)
	}

	for _, email := range emails {
		f.linkedAccounts[email.GetEmail()] = emails
	}
}

///////////////////////////////////////////////////////////////////////////////
// Helpers

func (c *Change) key() string {
	return key(c.Host, int(c.Info.GetNumber()))
}

func key(host string, change int) string {
	return fmt.Sprintf("%s/%d", host, change)
}

func psKey(host string, change, ps int) string {
	return fmt.Sprintf("%s/%d/%d", host, change, ps)
}

func splitPSKey(k string) (key string, ps int) {
	i := strings.LastIndex(k, "/")
	return k[:i], atoi(k[i+1:])
}

func (c *Change) resolveRevision(r string) (int, *gerritpb.RevisionInfo, error) {
	if ri, ok := c.Info.GetRevisions()[r]; ok {
		return int(ri.GetNumber()), ri, nil
	}
	if ps, err := strconv.Atoi(r); err == nil {
		_, ri := c.findRevisionForPS(ps)
		if ri != nil {
			return ps, ri, nil
		}
	}
	return 0, nil, status.Errorf(codes.NotFound,
		"couldn't resolve change %d revision %q", c.Info.GetNumber(), r)
}

func (c *Change) findRevisionForPS(ps int) (rev string, ri *gerritpb.RevisionInfo) {
	for rev, ri := range c.Info.GetRevisions() {
		if ri.GetNumber() == int32(ps) {
			return rev, ri
		}
	}
	return "", nil
}

func atoi64(s string) int64 {
	a, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(fmt.Errorf("invalid int %q: %s", s, err))
	}
	return a
}

func atoi(s string) int {
	return int(atoi64(s))
}

func parseChangePatchset(s any) (int, int) {
	switch v := s.(type) {
	case *gerritpb.ChangeInfo:
		return int(v.GetNumber()), int(v.GetRevisions()[v.GetCurrentRevision()].GetNumber())
	case *Change:
		return parseChangePatchset(v.Info)
	case string:
		if j := strings.IndexRune(v, '_'); j != -1 {
			return int(atoi64(v[:j])), int(atoi64(v[j+1:]))
		}
		panic(fmt.Errorf("unsupported %q: use change_patchset e.g. 123_1", v))
	default:
		panic(fmt.Errorf("unsupported type %T %v as change patchset", s, v))
	}
}

func (f *Fake) resolvePSKeyLocked(psk string) (ch *Change, rev string, ri *gerritpb.RevisionInfo, err error) {
	k, ps := splitPSKey(psk)
	var ok bool
	ch, ok = f.cs[k]
	if !ok {
		err = status.Errorf(codes.Unknown, "fake relation chain invalid: missing %s change", k)
		return
	}
	rev, ri = ch.findRevisionForPS(ps)
	if ri == nil {
		err = status.Errorf(codes.Unknown, "fake relation chain invalid: missing patchset %d for %s change", ps, k)
	}
	return
}
