// DO NOT EDIT!!!
// Copied from https://chromium.googlesource.com/infra/luci/recipes-py/+/HEAD/recipe_engine/recipes_cfg.proto
// Please run the update script (update/main.go) to pull the latest proto.

// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/recipes_py/proto/recipes_cfg.proto

package recipespb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type AutorollRecipeOptions_TrivialOptions_SelfApproveMethod int32

const (
	// Will set Bot-Commit+1 label.
	AutorollRecipeOptions_TrivialOptions_BOT_COMMIT_APPROVE AutorollRecipeOptions_TrivialOptions_SelfApproveMethod = 0
	// Will set Code-Review+1 label.
	AutorollRecipeOptions_TrivialOptions_CODE_REVIEW_1_APPROVE AutorollRecipeOptions_TrivialOptions_SelfApproveMethod = 1
	// Will set Code-Review+2 label.
	AutorollRecipeOptions_TrivialOptions_CODE_REVIEW_2_APPROVE AutorollRecipeOptions_TrivialOptions_SelfApproveMethod = 2
	// Will not set any labels besides Commit-Queue.
	AutorollRecipeOptions_TrivialOptions_NO_LABELS_APPROVE AutorollRecipeOptions_TrivialOptions_SelfApproveMethod = 3
)

// Enum value maps for AutorollRecipeOptions_TrivialOptions_SelfApproveMethod.
var (
	AutorollRecipeOptions_TrivialOptions_SelfApproveMethod_name = map[int32]string{
		0: "BOT_COMMIT_APPROVE",
		1: "CODE_REVIEW_1_APPROVE",
		2: "CODE_REVIEW_2_APPROVE",
		3: "NO_LABELS_APPROVE",
	}
	AutorollRecipeOptions_TrivialOptions_SelfApproveMethod_value = map[string]int32{
		"BOT_COMMIT_APPROVE":    0,
		"CODE_REVIEW_1_APPROVE": 1,
		"CODE_REVIEW_2_APPROVE": 2,
		"NO_LABELS_APPROVE":     3,
	}
)

func (x AutorollRecipeOptions_TrivialOptions_SelfApproveMethod) Enum() *AutorollRecipeOptions_TrivialOptions_SelfApproveMethod {
	p := new(AutorollRecipeOptions_TrivialOptions_SelfApproveMethod)
	*p = x
	return p
}

func (x AutorollRecipeOptions_TrivialOptions_SelfApproveMethod) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (AutorollRecipeOptions_TrivialOptions_SelfApproveMethod) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_enumTypes[0].Descriptor()
}

func (AutorollRecipeOptions_TrivialOptions_SelfApproveMethod) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_enumTypes[0]
}

func (x AutorollRecipeOptions_TrivialOptions_SelfApproveMethod) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use AutorollRecipeOptions_TrivialOptions_SelfApproveMethod.Descriptor instead.
func (AutorollRecipeOptions_TrivialOptions_SelfApproveMethod) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescGZIP(), []int{1, 0, 0}
}

type DepSpec struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// (required) The URL of where to fetch the repo. Must be a valid git URL.
	//
	// If you wish to avoid network/git operations, please use the `-O` override
	// functionality of recipes.py. See also `recipes.py bundle`.
	Url string `protobuf:"bytes,1,opt,name=url,proto3" json:"url,omitempty"`
	// (required) The ref to git-fetch when syncing this dependency.
	//
	// This must be an absolute ref which the server at `url` recognizes (e.g.
	// 'refs/heads/...').
	//
	// DEPRECATED: Short refs (like 'main') will be implicitly converted to
	// 'refs/heads/...' with a warning.
	Branch string `protobuf:"bytes,2,opt,name=branch,proto3" json:"branch,omitempty"`
	// (required) The git commit that we depend on.
	Revision      string `protobuf:"bytes,3,opt,name=revision,proto3" json:"revision,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DepSpec) Reset() {
	*x = DepSpec{}
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DepSpec) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DepSpec) ProtoMessage() {}

func (x *DepSpec) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DepSpec.ProtoReflect.Descriptor instead.
func (*DepSpec) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescGZIP(), []int{0}
}

func (x *DepSpec) GetUrl() string {
	if x != nil {
		return x.Url
	}
	return ""
}

func (x *DepSpec) GetBranch() string {
	if x != nil {
		return x.Branch
	}
	return ""
}

func (x *DepSpec) GetRevision() string {
	if x != nil {
		return x.Revision
	}
	return ""
}

// These options control the behavior of the autoroller recipe:
//
//	https://chromium.googlesource.com/infra/infra/+/main/recipes/recipes/recipe_autoroller.py
type AutorollRecipeOptions struct {
	state      protoimpl.MessageState                   `protogen:"open.v1"`
	Trivial    *AutorollRecipeOptions_TrivialOptions    `protobuf:"bytes,1,opt,name=trivial,proto3" json:"trivial,omitempty"`
	Nontrivial *AutorollRecipeOptions_NontrivialOptions `protobuf:"bytes,2,opt,name=nontrivial,proto3" json:"nontrivial,omitempty"`
	// Make the autoroller skip this repo entirely with a human-readable message.
	DisableReason string `protobuf:"bytes,3,opt,name=disable_reason,json=disableReason,proto3" json:"disable_reason,omitempty"`
	// If true, skip the autoroller will skip CCing authors of CLs in this repo
	// when rolling those CLs downstream.
	NoCcAuthors bool `protobuf:"varint,4,opt,name=no_cc_authors,json=noCcAuthors,proto3" json:"no_cc_authors,omitempty"`
	// If true, don't use the --r-owner option to 'git cl upload'.
	NoOwners      bool `protobuf:"varint,5,opt,name=no_owners,json=noOwners,proto3" json:"no_owners,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *AutorollRecipeOptions) Reset() {
	*x = AutorollRecipeOptions{}
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *AutorollRecipeOptions) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AutorollRecipeOptions) ProtoMessage() {}

func (x *AutorollRecipeOptions) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AutorollRecipeOptions.ProtoReflect.Descriptor instead.
func (*AutorollRecipeOptions) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescGZIP(), []int{1}
}

func (x *AutorollRecipeOptions) GetTrivial() *AutorollRecipeOptions_TrivialOptions {
	if x != nil {
		return x.Trivial
	}
	return nil
}

func (x *AutorollRecipeOptions) GetNontrivial() *AutorollRecipeOptions_NontrivialOptions {
	if x != nil {
		return x.Nontrivial
	}
	return nil
}

func (x *AutorollRecipeOptions) GetDisableReason() string {
	if x != nil {
		return x.DisableReason
	}
	return ""
}

func (x *AutorollRecipeOptions) GetNoCcAuthors() bool {
	if x != nil {
		return x.NoCcAuthors
	}
	return false
}

func (x *AutorollRecipeOptions) GetNoOwners() bool {
	if x != nil {
		return x.NoOwners
	}
	return false
}

type RepoSpec struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The "API Version" of this proto. Should always equal 2, currently.
	ApiVersion int32 `protobuf:"varint,1,opt,name=api_version,json=apiVersion,proto3" json:"api_version,omitempty"` // Version 2
	// The "repo name" of this recipe repository. This becomes
	// the prefix in DEPS when something depends on one of this repo's modules
	// (e.g.  DEPS=["recipe_engine/path"]).
	//
	// By convention, this should match the luci-config project_id for this repo,
	// but the only requirements are that:
	//   - It is unique within its recipes microcosm (i.e. no dependency tree of
	//     recipe repos can ever have two repos with the same name).
	//   - It must not contain slashes.
	//
	// One of 'repo_name' and 'project_id' (the old field) must be specified;
	// 'repo_name' takes precedence, and the autoroller will upgrade all
	// recipes.cfg files to have both. Eventually we will remove 'project_id'.
	RepoName string `protobuf:"bytes,7,opt,name=repo_name,json=repoName,proto3" json:"repo_name,omitempty"`
	// Deprecated: The old field for specifying repo_name.
	ProjectId string `protobuf:"bytes,2,opt,name=project_id,json=projectId,proto3" json:"project_id,omitempty"`
	// This is the URL which points to the 'source of truth' for this repo. It's
	// meant to be used for documentation generation.
	CanonicalRepoUrl string `protobuf:"bytes,3,opt,name=canonical_repo_url,json=canonicalRepoUrl,proto3" json:"canonical_repo_url,omitempty"`
	// The path (using forward slashes) to where the base of the recipes are found
	// in the repo (i.e. where the "recipes" and/or "recipe_modules" directories
	// live).
	RecipesPath string `protobuf:"bytes,4,opt,name=recipes_path,json=recipesPath,proto3" json:"recipes_path,omitempty"`
	// A mapping of a dependency ("repo_name") to spec needed to fetch its code.
	Deps map[string]*DepSpec `protobuf:"bytes,5,rep,name=deps,proto3" json:"deps,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	// The autoroller options for this repo. These options will be respected by
	// the autoroller recipe (which currently lives here:
	//
	//	https://chromium.googlesource.com/infra/infra/+/main/recipes/recipes/recipe_autoroller.py
	//
	// ).
	AutorollRecipeOptions *AutorollRecipeOptions `protobuf:"bytes,6,opt,name=autoroll_recipe_options,json=autorollRecipeOptions,proto3" json:"autoroll_recipe_options,omitempty"`
	// If true, `recipes.py test train` will not generate the README.recipes.md
	// docs file, and `recipes.py test run` will not assert that the docs file is
	// up-to-date.
	NoDocs bool `protobuf:"varint,8,opt,name=no_docs,json=noDocs,proto3" json:"no_docs,omitempty"`
	// DEPRECATED; python3 is always required.
	RequirePy3Compatibility bool `protobuf:"varint,9,opt,name=require_py3_compatibility,json=requirePy3Compatibility,proto3" json:"require_py3_compatibility,omitempty"`
	// DEPRECATED; python3 is always used.
	Py3Only bool `protobuf:"varint,10,opt,name=py3_only,json=py3Only,proto3" json:"py3_only,omitempty"`
	// If true, this repo will not allow any tests to exit with a status code that
	// mismatches the expected status of the `test` kwarg of the test data (e.g.
	// `yield api.test("name", ..., status="FAILURE")`), which defaults to SUCCESS
	// if not specified.
	EnforceTestExpectedStatus bool `protobuf:"varint,11,opt,name=enforce_test_expected_status,json=enforceTestExpectedStatus,proto3" json:"enforce_test_expected_status,omitempty"`
	// This is a list of recipe warnings which will be treated as errors. The list
	// items must always be in the form of "repo_name/WARNING_NAME", e.g.
	// "recipe_engine/CQ_MODULE_DEPRECATED".
	//
	// If a warning in this list is emitted when running the tests (e.g.
	// `recipes.py test XXX`) it will be logged as an error and will cause the
	// test command to return 1 instead of 0.
	//
	// If a warning in this list doesn't exist (e.g. the warning was removed from
	// the upstream repo), it will be reported with `recipes.py test XXX` that it
	// can be safely removed from this list.
	ForbiddenWarnings []string `protobuf:"bytes,12,rep,name=forbidden_warnings,json=forbiddenWarnings,proto3" json:"forbidden_warnings,omitempty"`
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *RepoSpec) Reset() {
	*x = RepoSpec{}
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RepoSpec) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RepoSpec) ProtoMessage() {}

func (x *RepoSpec) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RepoSpec.ProtoReflect.Descriptor instead.
func (*RepoSpec) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescGZIP(), []int{2}
}

func (x *RepoSpec) GetApiVersion() int32 {
	if x != nil {
		return x.ApiVersion
	}
	return 0
}

func (x *RepoSpec) GetRepoName() string {
	if x != nil {
		return x.RepoName
	}
	return ""
}

func (x *RepoSpec) GetProjectId() string {
	if x != nil {
		return x.ProjectId
	}
	return ""
}

func (x *RepoSpec) GetCanonicalRepoUrl() string {
	if x != nil {
		return x.CanonicalRepoUrl
	}
	return ""
}

func (x *RepoSpec) GetRecipesPath() string {
	if x != nil {
		return x.RecipesPath
	}
	return ""
}

func (x *RepoSpec) GetDeps() map[string]*DepSpec {
	if x != nil {
		return x.Deps
	}
	return nil
}

func (x *RepoSpec) GetAutorollRecipeOptions() *AutorollRecipeOptions {
	if x != nil {
		return x.AutorollRecipeOptions
	}
	return nil
}

func (x *RepoSpec) GetNoDocs() bool {
	if x != nil {
		return x.NoDocs
	}
	return false
}

func (x *RepoSpec) GetRequirePy3Compatibility() bool {
	if x != nil {
		return x.RequirePy3Compatibility
	}
	return false
}

func (x *RepoSpec) GetPy3Only() bool {
	if x != nil {
		return x.Py3Only
	}
	return false
}

func (x *RepoSpec) GetEnforceTestExpectedStatus() bool {
	if x != nil {
		return x.EnforceTestExpectedStatus
	}
	return false
}

func (x *RepoSpec) GetForbiddenWarnings() []string {
	if x != nil {
		return x.ForbiddenWarnings
	}
	return nil
}

// Emitted by the `recipes.py dump_specs` command.
type DepRepoSpecs struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	RepoSpecs     map[string]*RepoSpec   `protobuf:"bytes,1,rep,name=repo_specs,json=repoSpecs,proto3" json:"repo_specs,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DepRepoSpecs) Reset() {
	*x = DepRepoSpecs{}
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DepRepoSpecs) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DepRepoSpecs) ProtoMessage() {}

func (x *DepRepoSpecs) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DepRepoSpecs.ProtoReflect.Descriptor instead.
func (*DepRepoSpecs) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescGZIP(), []int{3}
}

func (x *DepRepoSpecs) GetRepoSpecs() map[string]*RepoSpec {
	if x != nil {
		return x.RepoSpecs
	}
	return nil
}

// These control the behavior of the autoroller when it finds a trivial roll
// (i.e. a roll without expectation changes).
type AutorollRecipeOptions_TrivialOptions struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// One of these email addresses will be randomly selected to be TBR'd.
	TbrEmails []string `protobuf:"bytes,1,rep,name=tbr_emails,json=tbrEmails,proto3" json:"tbr_emails,omitempty"`
	// If true, the autoroller recipe will automatically CQ the change.
	AutomaticCommit bool `protobuf:"varint,2,opt,name=automatic_commit,json=automaticCommit,proto3" json:"automatic_commit,omitempty"`
	// If true and automatic_commit is false, the autoroller recipe will
	// automatically do a CQ dry run when uploading the change.
	DryRun            bool                                                   `protobuf:"varint,3,opt,name=dry_run,json=dryRun,proto3" json:"dry_run,omitempty"`
	SelfApproveMethod AutorollRecipeOptions_TrivialOptions_SelfApproveMethod `protobuf:"varint,4,opt,name=self_approve_method,json=selfApproveMethod,proto3,enum=recipe_engine.AutorollRecipeOptions_TrivialOptions_SelfApproveMethod" json:"self_approve_method,omitempty"`
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *AutorollRecipeOptions_TrivialOptions) Reset() {
	*x = AutorollRecipeOptions_TrivialOptions{}
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *AutorollRecipeOptions_TrivialOptions) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AutorollRecipeOptions_TrivialOptions) ProtoMessage() {}

func (x *AutorollRecipeOptions_TrivialOptions) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AutorollRecipeOptions_TrivialOptions.ProtoReflect.Descriptor instead.
func (*AutorollRecipeOptions_TrivialOptions) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescGZIP(), []int{1, 0}
}

func (x *AutorollRecipeOptions_TrivialOptions) GetTbrEmails() []string {
	if x != nil {
		return x.TbrEmails
	}
	return nil
}

func (x *AutorollRecipeOptions_TrivialOptions) GetAutomaticCommit() bool {
	if x != nil {
		return x.AutomaticCommit
	}
	return false
}

func (x *AutorollRecipeOptions_TrivialOptions) GetDryRun() bool {
	if x != nil {
		return x.DryRun
	}
	return false
}

func (x *AutorollRecipeOptions_TrivialOptions) GetSelfApproveMethod() AutorollRecipeOptions_TrivialOptions_SelfApproveMethod {
	if x != nil {
		return x.SelfApproveMethod
	}
	return AutorollRecipeOptions_TrivialOptions_BOT_COMMIT_APPROVE
}

// These control the behavior of the autoroller when it finds a non-trivial
// roll (i.e. a roll with expectation changes but which otherwise completes
// the simulation tests).
type AutorollRecipeOptions_NontrivialOptions struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// These add additional reviewer emails on the change.
	ExtraReviewerEmails []string `protobuf:"bytes,1,rep,name=extra_reviewer_emails,json=extraReviewerEmails,proto3" json:"extra_reviewer_emails,omitempty"`
	// If true, the autoroller recipe will automatically do a CQ dry run when
	// uploading the change.
	AutomaticCommitDryRun bool `protobuf:"varint,2,opt,name=automatic_commit_dry_run,json=automaticCommitDryRun,proto3" json:"automatic_commit_dry_run,omitempty"`
	// If true, the autoroller recipe will set the Auto-Submit+1 label when
	// uploading the change. This should only be enabled for projects which
	// support Auto-Submit.
	SetAutosubmit bool `protobuf:"varint,3,opt,name=set_autosubmit,json=setAutosubmit,proto3" json:"set_autosubmit,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *AutorollRecipeOptions_NontrivialOptions) Reset() {
	*x = AutorollRecipeOptions_NontrivialOptions{}
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *AutorollRecipeOptions_NontrivialOptions) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AutorollRecipeOptions_NontrivialOptions) ProtoMessage() {}

func (x *AutorollRecipeOptions_NontrivialOptions) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AutorollRecipeOptions_NontrivialOptions.ProtoReflect.Descriptor instead.
func (*AutorollRecipeOptions_NontrivialOptions) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescGZIP(), []int{1, 1}
}

func (x *AutorollRecipeOptions_NontrivialOptions) GetExtraReviewerEmails() []string {
	if x != nil {
		return x.ExtraReviewerEmails
	}
	return nil
}

func (x *AutorollRecipeOptions_NontrivialOptions) GetAutomaticCommitDryRun() bool {
	if x != nil {
		return x.AutomaticCommitDryRun
	}
	return false
}

func (x *AutorollRecipeOptions_NontrivialOptions) GetSetAutosubmit() bool {
	if x != nil {
		return x.SetAutosubmit
	}
	return false
}

var File_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDesc = string([]byte{
	0x0a, 0x37, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x73, 0x5f, 0x70,
	0x79, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x73, 0x5f,
	0x63, 0x66, 0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0d, 0x72, 0x65, 0x63, 0x69, 0x70,
	0x65, 0x5f, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x22, 0x4f, 0x0a, 0x07, 0x44, 0x65, 0x70, 0x53,
	0x70, 0x65, 0x63, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x72, 0x6c, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x03, 0x75, 0x72, 0x6c, 0x12, 0x16, 0x0a, 0x06, 0x62, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x62, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x12, 0x1a, 0x0a,
	0x08, 0x72, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x08, 0x72, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x22, 0xb7, 0x06, 0x0a, 0x15, 0x41, 0x75,
	0x74, 0x6f, 0x72, 0x6f, 0x6c, 0x6c, 0x52, 0x65, 0x63, 0x69, 0x70, 0x65, 0x4f, 0x70, 0x74, 0x69,
	0x6f, 0x6e, 0x73, 0x12, 0x4d, 0x0a, 0x07, 0x74, 0x72, 0x69, 0x76, 0x69, 0x61, 0x6c, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x33, 0x2e, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x5f, 0x65, 0x6e,
	0x67, 0x69, 0x6e, 0x65, 0x2e, 0x41, 0x75, 0x74, 0x6f, 0x72, 0x6f, 0x6c, 0x6c, 0x52, 0x65, 0x63,
	0x69, 0x70, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x54, 0x72, 0x69, 0x76, 0x69,
	0x61, 0x6c, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x07, 0x74, 0x72, 0x69, 0x76, 0x69,
	0x61, 0x6c, 0x12, 0x56, 0x0a, 0x0a, 0x6e, 0x6f, 0x6e, 0x74, 0x72, 0x69, 0x76, 0x69, 0x61, 0x6c,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x36, 0x2e, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x5f,
	0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x41, 0x75, 0x74, 0x6f, 0x72, 0x6f, 0x6c, 0x6c, 0x52,
	0x65, 0x63, 0x69, 0x70, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x4e, 0x6f, 0x6e,
	0x74, 0x72, 0x69, 0x76, 0x69, 0x61, 0x6c, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x0a,
	0x6e, 0x6f, 0x6e, 0x74, 0x72, 0x69, 0x76, 0x69, 0x61, 0x6c, 0x12, 0x25, 0x0a, 0x0e, 0x64, 0x69,
	0x73, 0x61, 0x62, 0x6c, 0x65, 0x5f, 0x72, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0d, 0x64, 0x69, 0x73, 0x61, 0x62, 0x6c, 0x65, 0x52, 0x65, 0x61, 0x73, 0x6f,
	0x6e, 0x12, 0x22, 0x0a, 0x0d, 0x6e, 0x6f, 0x5f, 0x63, 0x63, 0x5f, 0x61, 0x75, 0x74, 0x68, 0x6f,
	0x72, 0x73, 0x18, 0x04, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0b, 0x6e, 0x6f, 0x43, 0x63, 0x41, 0x75,
	0x74, 0x68, 0x6f, 0x72, 0x73, 0x12, 0x1b, 0x0a, 0x09, 0x6e, 0x6f, 0x5f, 0x6f, 0x77, 0x6e, 0x65,
	0x72, 0x73, 0x18, 0x05, 0x20, 0x01, 0x28, 0x08, 0x52, 0x08, 0x6e, 0x6f, 0x4f, 0x77, 0x6e, 0x65,
	0x72, 0x73, 0x1a, 0xe4, 0x02, 0x0a, 0x0e, 0x54, 0x72, 0x69, 0x76, 0x69, 0x61, 0x6c, 0x4f, 0x70,
	0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x1d, 0x0a, 0x0a, 0x74, 0x62, 0x72, 0x5f, 0x65, 0x6d, 0x61,
	0x69, 0x6c, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x09, 0x74, 0x62, 0x72, 0x45, 0x6d,
	0x61, 0x69, 0x6c, 0x73, 0x12, 0x29, 0x0a, 0x10, 0x61, 0x75, 0x74, 0x6f, 0x6d, 0x61, 0x74, 0x69,
	0x63, 0x5f, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0f,
	0x61, 0x75, 0x74, 0x6f, 0x6d, 0x61, 0x74, 0x69, 0x63, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x12,
	0x17, 0x0a, 0x07, 0x64, 0x72, 0x79, 0x5f, 0x72, 0x75, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x08,
	0x52, 0x06, 0x64, 0x72, 0x79, 0x52, 0x75, 0x6e, 0x12, 0x75, 0x0a, 0x13, 0x73, 0x65, 0x6c, 0x66,
	0x5f, 0x61, 0x70, 0x70, 0x72, 0x6f, 0x76, 0x65, 0x5f, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x45, 0x2e, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x5f, 0x65,
	0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x41, 0x75, 0x74, 0x6f, 0x72, 0x6f, 0x6c, 0x6c, 0x52, 0x65,
	0x63, 0x69, 0x70, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x54, 0x72, 0x69, 0x76,
	0x69, 0x61, 0x6c, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x53, 0x65, 0x6c, 0x66, 0x41,
	0x70, 0x70, 0x72, 0x6f, 0x76, 0x65, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x52, 0x11, 0x73, 0x65,
	0x6c, 0x66, 0x41, 0x70, 0x70, 0x72, 0x6f, 0x76, 0x65, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x22,
	0x78, 0x0a, 0x11, 0x53, 0x65, 0x6c, 0x66, 0x41, 0x70, 0x70, 0x72, 0x6f, 0x76, 0x65, 0x4d, 0x65,
	0x74, 0x68, 0x6f, 0x64, 0x12, 0x16, 0x0a, 0x12, 0x42, 0x4f, 0x54, 0x5f, 0x43, 0x4f, 0x4d, 0x4d,
	0x49, 0x54, 0x5f, 0x41, 0x50, 0x50, 0x52, 0x4f, 0x56, 0x45, 0x10, 0x00, 0x12, 0x19, 0x0a, 0x15,
	0x43, 0x4f, 0x44, 0x45, 0x5f, 0x52, 0x45, 0x56, 0x49, 0x45, 0x57, 0x5f, 0x31, 0x5f, 0x41, 0x50,
	0x50, 0x52, 0x4f, 0x56, 0x45, 0x10, 0x01, 0x12, 0x19, 0x0a, 0x15, 0x43, 0x4f, 0x44, 0x45, 0x5f,
	0x52, 0x45, 0x56, 0x49, 0x45, 0x57, 0x5f, 0x32, 0x5f, 0x41, 0x50, 0x50, 0x52, 0x4f, 0x56, 0x45,
	0x10, 0x02, 0x12, 0x15, 0x0a, 0x11, 0x4e, 0x4f, 0x5f, 0x4c, 0x41, 0x42, 0x45, 0x4c, 0x53, 0x5f,
	0x41, 0x50, 0x50, 0x52, 0x4f, 0x56, 0x45, 0x10, 0x03, 0x1a, 0xa7, 0x01, 0x0a, 0x11, 0x4e, 0x6f,
	0x6e, 0x74, 0x72, 0x69, 0x76, 0x69, 0x61, 0x6c, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12,
	0x32, 0x0a, 0x15, 0x65, 0x78, 0x74, 0x72, 0x61, 0x5f, 0x72, 0x65, 0x76, 0x69, 0x65, 0x77, 0x65,
	0x72, 0x5f, 0x65, 0x6d, 0x61, 0x69, 0x6c, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x13,
	0x65, 0x78, 0x74, 0x72, 0x61, 0x52, 0x65, 0x76, 0x69, 0x65, 0x77, 0x65, 0x72, 0x45, 0x6d, 0x61,
	0x69, 0x6c, 0x73, 0x12, 0x37, 0x0a, 0x18, 0x61, 0x75, 0x74, 0x6f, 0x6d, 0x61, 0x74, 0x69, 0x63,
	0x5f, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x5f, 0x64, 0x72, 0x79, 0x5f, 0x72, 0x75, 0x6e, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x08, 0x52, 0x15, 0x61, 0x75, 0x74, 0x6f, 0x6d, 0x61, 0x74, 0x69, 0x63,
	0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x44, 0x72, 0x79, 0x52, 0x75, 0x6e, 0x12, 0x25, 0x0a, 0x0e,
	0x73, 0x65, 0x74, 0x5f, 0x61, 0x75, 0x74, 0x6f, 0x73, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x08, 0x52, 0x0d, 0x73, 0x65, 0x74, 0x41, 0x75, 0x74, 0x6f, 0x73, 0x75, 0x62,
	0x6d, 0x69, 0x74, 0x22, 0xfe, 0x04, 0x0a, 0x08, 0x52, 0x65, 0x70, 0x6f, 0x53, 0x70, 0x65, 0x63,
	0x12, 0x1f, 0x0a, 0x0b, 0x61, 0x70, 0x69, 0x5f, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0a, 0x61, 0x70, 0x69, 0x56, 0x65, 0x72, 0x73, 0x69, 0x6f,
	0x6e, 0x12, 0x1b, 0x0a, 0x09, 0x72, 0x65, 0x70, 0x6f, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x07,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x72, 0x65, 0x70, 0x6f, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x1d,
	0x0a, 0x0a, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x09, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x49, 0x64, 0x12, 0x2c, 0x0a,
	0x12, 0x63, 0x61, 0x6e, 0x6f, 0x6e, 0x69, 0x63, 0x61, 0x6c, 0x5f, 0x72, 0x65, 0x70, 0x6f, 0x5f,
	0x75, 0x72, 0x6c, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x63, 0x61, 0x6e, 0x6f, 0x6e,
	0x69, 0x63, 0x61, 0x6c, 0x52, 0x65, 0x70, 0x6f, 0x55, 0x72, 0x6c, 0x12, 0x21, 0x0a, 0x0c, 0x72,
	0x65, 0x63, 0x69, 0x70, 0x65, 0x73, 0x5f, 0x70, 0x61, 0x74, 0x68, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0b, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x73, 0x50, 0x61, 0x74, 0x68, 0x12, 0x35,
	0x0a, 0x04, 0x64, 0x65, 0x70, 0x73, 0x18, 0x05, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x21, 0x2e, 0x72,
	0x65, 0x63, 0x69, 0x70, 0x65, 0x5f, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x52, 0x65, 0x70,
	0x6f, 0x53, 0x70, 0x65, 0x63, 0x2e, 0x44, 0x65, 0x70, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52,
	0x04, 0x64, 0x65, 0x70, 0x73, 0x12, 0x5c, 0x0a, 0x17, 0x61, 0x75, 0x74, 0x6f, 0x72, 0x6f, 0x6c,
	0x6c, 0x5f, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x5f, 0x6f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73,
	0x18, 0x06, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x24, 0x2e, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x5f,
	0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x41, 0x75, 0x74, 0x6f, 0x72, 0x6f, 0x6c, 0x6c, 0x52,
	0x65, 0x63, 0x69, 0x70, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x15, 0x61, 0x75,
	0x74, 0x6f, 0x72, 0x6f, 0x6c, 0x6c, 0x52, 0x65, 0x63, 0x69, 0x70, 0x65, 0x4f, 0x70, 0x74, 0x69,
	0x6f, 0x6e, 0x73, 0x12, 0x17, 0x0a, 0x07, 0x6e, 0x6f, 0x5f, 0x64, 0x6f, 0x63, 0x73, 0x18, 0x08,
	0x20, 0x01, 0x28, 0x08, 0x52, 0x06, 0x6e, 0x6f, 0x44, 0x6f, 0x63, 0x73, 0x12, 0x3a, 0x0a, 0x19,
	0x72, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x5f, 0x70, 0x79, 0x33, 0x5f, 0x63, 0x6f, 0x6d, 0x70,
	0x61, 0x74, 0x69, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x18, 0x09, 0x20, 0x01, 0x28, 0x08, 0x52,
	0x17, 0x72, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x50, 0x79, 0x33, 0x43, 0x6f, 0x6d, 0x70, 0x61,
	0x74, 0x69, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x79, 0x12, 0x19, 0x0a, 0x08, 0x70, 0x79, 0x33, 0x5f,
	0x6f, 0x6e, 0x6c, 0x79, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x70, 0x79, 0x33, 0x4f,
	0x6e, 0x6c, 0x79, 0x12, 0x3f, 0x0a, 0x1c, 0x65, 0x6e, 0x66, 0x6f, 0x72, 0x63, 0x65, 0x5f, 0x74,
	0x65, 0x73, 0x74, 0x5f, 0x65, 0x78, 0x70, 0x65, 0x63, 0x74, 0x65, 0x64, 0x5f, 0x73, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x08, 0x52, 0x19, 0x65, 0x6e, 0x66, 0x6f, 0x72,
	0x63, 0x65, 0x54, 0x65, 0x73, 0x74, 0x45, 0x78, 0x70, 0x65, 0x63, 0x74, 0x65, 0x64, 0x53, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x12, 0x2d, 0x0a, 0x12, 0x66, 0x6f, 0x72, 0x62, 0x69, 0x64, 0x64, 0x65,
	0x6e, 0x5f, 0x77, 0x61, 0x72, 0x6e, 0x69, 0x6e, 0x67, 0x73, 0x18, 0x0c, 0x20, 0x03, 0x28, 0x09,
	0x52, 0x11, 0x66, 0x6f, 0x72, 0x62, 0x69, 0x64, 0x64, 0x65, 0x6e, 0x57, 0x61, 0x72, 0x6e, 0x69,
	0x6e, 0x67, 0x73, 0x1a, 0x4f, 0x0a, 0x09, 0x44, 0x65, 0x70, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79,
	0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b,
	0x65, 0x79, 0x12, 0x2c, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x16, 0x2e, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x5f, 0x65, 0x6e, 0x67, 0x69, 0x6e,
	0x65, 0x2e, 0x44, 0x65, 0x70, 0x53, 0x70, 0x65, 0x63, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65,
	0x3a, 0x02, 0x38, 0x01, 0x22, 0xb0, 0x01, 0x0a, 0x0c, 0x44, 0x65, 0x70, 0x52, 0x65, 0x70, 0x6f,
	0x53, 0x70, 0x65, 0x63, 0x73, 0x12, 0x49, 0x0a, 0x0a, 0x72, 0x65, 0x70, 0x6f, 0x5f, 0x73, 0x70,
	0x65, 0x63, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2a, 0x2e, 0x72, 0x65, 0x63, 0x69,
	0x70, 0x65, 0x5f, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x44, 0x65, 0x70, 0x52, 0x65, 0x70,
	0x6f, 0x53, 0x70, 0x65, 0x63, 0x73, 0x2e, 0x52, 0x65, 0x70, 0x6f, 0x53, 0x70, 0x65, 0x63, 0x73,
	0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x09, 0x72, 0x65, 0x70, 0x6f, 0x53, 0x70, 0x65, 0x63, 0x73,
	0x1a, 0x55, 0x0a, 0x0e, 0x52, 0x65, 0x70, 0x6f, 0x53, 0x70, 0x65, 0x63, 0x73, 0x45, 0x6e, 0x74,
	0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x03, 0x6b, 0x65, 0x79, 0x12, 0x2d, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x5f, 0x65, 0x6e, 0x67,
	0x69, 0x6e, 0x65, 0x2e, 0x52, 0x65, 0x70, 0x6f, 0x53, 0x70, 0x65, 0x63, 0x52, 0x05, 0x76, 0x61,
	0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x42, 0x31, 0x5a, 0x2f, 0x67, 0x6f, 0x2e, 0x63, 0x68,
	0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f,
	0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x73, 0x5f, 0x70, 0x79, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x3b, 0x72, 0x65, 0x63, 0x69, 0x70, 0x65, 0x73, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescData []byte
)

func file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDesc), len(file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDescData
}

var file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes = make([]protoimpl.MessageInfo, 8)
var file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_goTypes = []any{
	(AutorollRecipeOptions_TrivialOptions_SelfApproveMethod)(0), // 0: recipe_engine.AutorollRecipeOptions.TrivialOptions.SelfApproveMethod
	(*DepSpec)(nil),                                 // 1: recipe_engine.DepSpec
	(*AutorollRecipeOptions)(nil),                   // 2: recipe_engine.AutorollRecipeOptions
	(*RepoSpec)(nil),                                // 3: recipe_engine.RepoSpec
	(*DepRepoSpecs)(nil),                            // 4: recipe_engine.DepRepoSpecs
	(*AutorollRecipeOptions_TrivialOptions)(nil),    // 5: recipe_engine.AutorollRecipeOptions.TrivialOptions
	(*AutorollRecipeOptions_NontrivialOptions)(nil), // 6: recipe_engine.AutorollRecipeOptions.NontrivialOptions
	nil, // 7: recipe_engine.RepoSpec.DepsEntry
	nil, // 8: recipe_engine.DepRepoSpecs.RepoSpecsEntry
}
var file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_depIdxs = []int32{
	5, // 0: recipe_engine.AutorollRecipeOptions.trivial:type_name -> recipe_engine.AutorollRecipeOptions.TrivialOptions
	6, // 1: recipe_engine.AutorollRecipeOptions.nontrivial:type_name -> recipe_engine.AutorollRecipeOptions.NontrivialOptions
	7, // 2: recipe_engine.RepoSpec.deps:type_name -> recipe_engine.RepoSpec.DepsEntry
	2, // 3: recipe_engine.RepoSpec.autoroll_recipe_options:type_name -> recipe_engine.AutorollRecipeOptions
	8, // 4: recipe_engine.DepRepoSpecs.repo_specs:type_name -> recipe_engine.DepRepoSpecs.RepoSpecsEntry
	0, // 5: recipe_engine.AutorollRecipeOptions.TrivialOptions.self_approve_method:type_name -> recipe_engine.AutorollRecipeOptions.TrivialOptions.SelfApproveMethod
	1, // 6: recipe_engine.RepoSpec.DepsEntry.value:type_name -> recipe_engine.DepSpec
	3, // 7: recipe_engine.DepRepoSpecs.RepoSpecsEntry.value:type_name -> recipe_engine.RepoSpec
	8, // [8:8] is the sub-list for method output_type
	8, // [8:8] is the sub-list for method input_type
	8, // [8:8] is the sub-list for extension type_name
	8, // [8:8] is the sub-list for extension extendee
	0, // [0:8] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_init() }
func file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_init() {
	if File_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDesc), len(file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   8,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto = out.File
	file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_goTypes = nil
	file_go_chromium_org_luci_recipes_py_proto_recipes_cfg_proto_depIdxs = nil
}
