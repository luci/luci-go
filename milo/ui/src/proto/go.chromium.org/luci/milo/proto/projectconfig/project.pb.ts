/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { BuilderID } from "../../../buildbucket/proto/builder_common.pb";

export const protobufPackage = "luci.milo.projectconfig";

/** Project is a project definition for Milo. */
export interface Project {
  /** Consoles is a list of consoles to define under /console/ */
  readonly consoles: readonly Console[];
  /** Headers is a list of defined headers that may be referenced by a console. */
  readonly headers: readonly Header[];
  /**
   * LogoUrl is the URL to the logo for this project.
   * This field is optional. The logo URL must have a host of
   * storage.googleapis.com.
   */
  readonly logoUrl: string;
  /**
   * IgnoredBuilderIds is a list of builder IDs to be ignored by pubsub handler.
   * Build update events from the builders in this list will be ignored to
   * improve performance.
   * This means that updates to these builders will not be reflected in
   * consoles, the builders page, or the builder groups page.
   * Builders that post updates frequently (> 1/s) should be added to this list.
   * ID format: <bucket>/<builder>
   *
   * TODO(crbug.com/1099036): deprecate this once a long term solution is
   * implemented.
   */
  readonly ignoredBuilderIds: readonly string[];
  /**
   * BugUrlTemplate is the template for making a custom bug link for
   * filing a bug against a build that displays on the build page. This
   * is a separate link from the general Milo feedback link that is
   * displayed in the top right corner. This field is optional.
   *
   * The protocal must be `https` and the domain name must be one of the
   * following:
   * * bugs.chromium.org
   * * b.corp.google.com
   *
   * The template is interpreted as a mustache template and the following
   * variables are available:
   * * {{{ build.builder.project }}}
   * * {{{ build.builder.bucket }}}
   * * {{{ build.builder.builder }}}
   * * {{{ milo_build_url }}}
   * * {{{ milo_builder_url }}}
   *
   * All variables are URL component encoded. Additionally, use `{{{ ... }}}` to
   * disable HTML escaping. If the template is not a valid mustache template, or
   * doesn't satisfy the requirements above, the link is not displayed.
   */
  readonly bugUrlTemplate: string;
  /** MetadataConfig is the display configuration for arbitrary metadata. */
  readonly metadataConfig: MetadataConfig | undefined;
}

/**
 * Link is a link to an internet resource, which will be rendered out as
 * an anchor tag <a href="url" alt="alt">text</a>.
 */
export interface Link {
  /** Text is displayed as the text between the anchor tags. */
  readonly text: string;
  /** Url is the URL to link to. */
  readonly url: string;
  /** Alt is the alt text displayed when hovering over the text. */
  readonly alt: string;
}

/**
 * Oncall contains information about who is currently scheduled as the
 * oncall (Sheriff, trooper, etc) for certain rotations.
 */
export interface Oncall {
  /** Name is the name of the oncall rotation being displayed. */
  readonly name: string;
  /**
   * Url is an URL to a json endpoint with the following format:
   * {
   *   "updated_unix_timestamp": <int>,
   *   "emails": [
   *     "email@somewhere.com",
   *     "email@nowhere.com
   *   ]
   * }
   */
  readonly url: string;
  /**
   * ShowPrimarySecondaryLabels specifies whether the oncaller names
   * should have "(primary)" and "(secondary)" appended to them if there
   * are more than one.
   */
  readonly showPrimarySecondaryLabels: boolean;
}

/** LinkGroup is a list of links, optionally given a name. */
export interface LinkGroup {
  /** Name is the name of this list of links. This is optional. */
  readonly name: string;
  /** Links is a list of links to display. */
  readonly links: readonly Link[];
}

/**
 * ConsoleSummaryGroup is a list of consoles to be displayed as console summaries
 * (aka the little bubbles at the top of the console).  This can optionally
 * have a group name if specified in the group_link.
 * (e.g. "Tree closers", "Experimental", etc)
 */
export interface ConsoleSummaryGroup {
  /** Title is a name or label for this group of consoles.  This is optional. */
  readonly title:
    | Link
    | undefined;
  /**
   * ConsoleIds is a list of console ids to display in this console group.
   * Each console id must be prepended with its related project (e.g.
   * chromium/main) because console ids are project-local.
   * Only consoles from the same project are supported.
   * TODO(hinoka): Allow cross-project consoles.
   */
  readonly consoleIds: readonly string[];
}

/**
 * Header is a collection of links, rotation information, and console summaries
 * that are displayed at the top of a console, below the tree status information.
 * Links and oncall information is always laid out to the left, while
 * console groups are laid out on the right.  Each oncall and links group
 * take up a row.
 */
export interface Header {
  /**
   * Oncalls are a reference to oncall rotations, which is a URL to a json
   * endpoint with the following format:
   * {
   *   "updated_unix_timestamp": <int>,
   *   "emails": [
   *     "email@somewhere.com",
   *     "email@nowhere.com
   *   ]
   * }
   */
  readonly oncalls: readonly Oncall[];
  /** Links is a list of named groups of web links. */
  readonly links: readonly LinkGroup[];
  /** ConsoleGroups are groups of console summaries, each optionally named. */
  readonly consoleGroups: readonly ConsoleSummaryGroup[];
  /**
   * TreeStatusHost is the hostname of the chromium-status instance where
   * the tree status of this console is hosted.  If provided, this will appear
   * as the bar at the very top of the page.
   */
  readonly treeStatusHost: string;
  /** Id is a reference to the header. */
  readonly id: string;
}

/** Console is a waterfall definition consisting of one or more builders. */
export interface Console {
  /**
   * Id is the reference to the console. The console will be visible at the
   * following URL: /p/<Project>/g/<ID>/console.
   */
  readonly id: string;
  /**
   * Name is the longform name of the waterfall, and will be used to be
   * displayed in the title.
   */
  readonly name: string;
  /** RepoUrl is the URL of the git repository to display as the rows of the console. */
  readonly repoUrl: string;
  /**
   * Refs are the refs to pull commits from when displaying the console.
   *
   * Users can specify a regular expression to match several refs using
   * "regexp:" prefix, but the regular expression must have:
   *   * a literal prefix with at least two slashes present, e.g.
   *     "refs/release-\d+/foobar" is not allowed, because the literal prefix
   *     "refs/release-" only contains one slash, and
   *   * must not start with ^ or end with $ as they are added automatically.
   *
   * For best results, ensure each ref's has commit's **committer** timestamp
   * monotonically non-decreasing. Gerrit will take care of this if you require
   * each commmit to go through Gerrit by prohibiting "git push" on these refs.
   *
   * Eg. refs/heads/main, regexp:refs/branch-heads/\d+\.\d+
   */
  readonly refs: readonly string[];
  /**
   * ExcludeRef is a ref, commits from which are ignored even when they are
   * reachable from the ref specified above. This must be specified as a single
   * fully-qualified ref, i.e. regexp syntax from above is not supported.
   *
   * Note: force pushes to this ref are not supported. Milo uses caching
   * assuming set of commits reachable from this ref may only grow, never lose
   * some commits.
   *
   * E.g. the config below allows to track commits from all release branches,
   * but ignore the commits from the main branch, from which these release
   * branches are branched off:
   *   ref: "regexp:refs/branch-heads/\d+\.\d+"
   *   exlude_ref: "refs/heads/main"
   */
  readonly excludeRef: string;
  /**
   * ManifestName the name of the manifest the waterfall looks at.
   * This should always be "REVISION".
   * In the future, other manifest names can be supported.
   * TODO(hinoka,iannucci): crbug/832893 - Support custom manifest names, such as "UNPATCHED" / "PATCHED".
   */
  readonly manifestName: string;
  /** Builders is a list of builder configurations to display as the columns of the console. */
  readonly builders: readonly Builder[];
  /**
   * FaviconUrl is the URL to the favicon for this console page.
   * This field is optional. The favicon URL must have a host of
   * storage.googleapis.com.
   */
  readonly faviconUrl: string;
  /**
   * Header is a collection of links, rotation information, and console summaries
   * displayed under the tree status but above the main console content.
   */
  readonly header:
    | Header
    | undefined;
  /**
   * HeaderId is a reference to a header.  Only one of Header or HeaderId should
   * be specified.
   */
  readonly headerId: string;
  /**
   * If true, this console will not filter out builds marked as Experimental.
   * This field is optional. By default Consoles only show production builds.
   */
  readonly includeExperimentalBuilds: boolean;
  /**
   * If true, only builders view will be available. Console view (i.e. git log
   * based view) will be disabled and users redirected to builder view.
   * Defaults to false.
   */
  readonly builderViewOnly: boolean;
  /**
   * If set, will change the default number of commits to query on a single page.
   * If not set, will default to 50.
   */
  readonly defaultCommitLimit: number;
  /**
   * If set, will default the console page to expanded view.
   * If not set, will default to collapsed view.
   */
  readonly defaultExpand: boolean;
  /**
   * ExternalProject indicates that this is not a new console, but an external
   * one that should be included from the specified project.
   * If this is set, ExternalId must also be set, and no other fields must be
   * set except for Id and Name.
   */
  readonly externalProject: string;
  /**
   * ExternalId indicates that this is not a new console, but an external one
   * with the specified ID that should be included from ExternalProject.
   * If this is set, ExternalProject must also be set, and no other fields must
   * be set except for Id and Name.
   */
  readonly externalId: string;
  /**
   * Realm that the console exists under.
   * See https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/appengine/auth_service/proto/realms_config.proto
   */
  readonly realm: string;
}

/** Builder is a reference to a Milo builder. */
export interface Builder {
  /**
   * Deprecated: use `id` instead.
   *
   * Name is the BuilderID of the builders you wish to display for this column
   * in the console. e.g.
   *   * "buildbucket/luci.chromium.try/linux_chromium_rel_ng"
   */
  readonly name: string;
  /**
   * Id is the ID of the builder.
   * If not defined, fallbacks to parsing the ID from `name`.
   */
  readonly id:
    | BuilderID
    | undefined;
  /**
   * Category describes the hierarchy of the builder on the header of the
   * console as a "|" delimited list.  Neighboring builders with common ancestors
   * will be have their headers merged.
   * In expanded view, each leaf category OR builder under a non-leaf category
   * will have it's own column.  The recommendation for maximum densification
   * is not to mix subcategories and builders for children of each category.
   */
  readonly category: string;
  /**
   * ShortName is shorter name of the builder.
   * The recommendation is to keep this name as short as reasonable,
   * as longer names take up more horizontal space.
   */
  readonly shortName: string;
}

/** MetadataConfig specifies how to display arbitrary metadata. */
export interface MetadataConfig {
  /**
   * TestMetadataProperties specify how data in luci.resultdb.v1.TestMetadata.Properties
   * should be presented in the UI.
   */
  readonly testMetadataProperties: readonly DisplayRule[];
}

/**
 * DisplayRule selects fields from a JSON object to present,
 * given the schema of the JSON object.
 */
export interface DisplayRule {
  /**
   * The schema of the JSON object, which the DisplayItems rules apply.
   * Use the fully-qualified name of the source protocol buffer.
   * eg. chromiumos.test.api.TestCaseMetadata
   */
  readonly schema: string;
  /** A list of items to display. */
  readonly displayItems: readonly DisplayItem[];
}

/** DisplayItem is the information about the item to display. */
export interface DisplayItem {
  /** A user-friendly name to describe the item. */
  readonly displayName: string;
  /**
   * The path to the item from the root of the JSON object.
   * Only support object walking to access nested objects.
   * Eg. object: {a:{b:1}}, path: a.b, return: 1
   */
  readonly path: string;
}

function createBaseProject(): Project {
  return {
    consoles: [],
    headers: [],
    logoUrl: "",
    ignoredBuilderIds: [],
    bugUrlTemplate: "",
    metadataConfig: undefined,
  };
}

export const Project = {
  encode(message: Project, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.consoles) {
      Console.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.headers) {
      Header.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    if (message.logoUrl !== "") {
      writer.uint32(34).string(message.logoUrl);
    }
    for (const v of message.ignoredBuilderIds) {
      writer.uint32(58).string(v!);
    }
    if (message.bugUrlTemplate !== "") {
      writer.uint32(66).string(message.bugUrlTemplate);
    }
    if (message.metadataConfig !== undefined) {
      MetadataConfig.encode(message.metadataConfig, writer.uint32(74).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Project {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProject() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 2:
          if (tag !== 18) {
            break;
          }

          message.consoles.push(Console.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.headers.push(Header.decode(reader, reader.uint32()));
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.logoUrl = reader.string();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.ignoredBuilderIds.push(reader.string());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.bugUrlTemplate = reader.string();
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.metadataConfig = MetadataConfig.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Project {
    return {
      consoles: globalThis.Array.isArray(object?.consoles) ? object.consoles.map((e: any) => Console.fromJSON(e)) : [],
      headers: globalThis.Array.isArray(object?.headers) ? object.headers.map((e: any) => Header.fromJSON(e)) : [],
      logoUrl: isSet(object.logoUrl) ? globalThis.String(object.logoUrl) : "",
      ignoredBuilderIds: globalThis.Array.isArray(object?.ignoredBuilderIds)
        ? object.ignoredBuilderIds.map((e: any) => globalThis.String(e))
        : [],
      bugUrlTemplate: isSet(object.bugUrlTemplate) ? globalThis.String(object.bugUrlTemplate) : "",
      metadataConfig: isSet(object.metadataConfig) ? MetadataConfig.fromJSON(object.metadataConfig) : undefined,
    };
  },

  toJSON(message: Project): unknown {
    const obj: any = {};
    if (message.consoles?.length) {
      obj.consoles = message.consoles.map((e) => Console.toJSON(e));
    }
    if (message.headers?.length) {
      obj.headers = message.headers.map((e) => Header.toJSON(e));
    }
    if (message.logoUrl !== "") {
      obj.logoUrl = message.logoUrl;
    }
    if (message.ignoredBuilderIds?.length) {
      obj.ignoredBuilderIds = message.ignoredBuilderIds;
    }
    if (message.bugUrlTemplate !== "") {
      obj.bugUrlTemplate = message.bugUrlTemplate;
    }
    if (message.metadataConfig !== undefined) {
      obj.metadataConfig = MetadataConfig.toJSON(message.metadataConfig);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Project>, I>>(base?: I): Project {
    return Project.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Project>, I>>(object: I): Project {
    const message = createBaseProject() as any;
    message.consoles = object.consoles?.map((e) => Console.fromPartial(e)) || [];
    message.headers = object.headers?.map((e) => Header.fromPartial(e)) || [];
    message.logoUrl = object.logoUrl ?? "";
    message.ignoredBuilderIds = object.ignoredBuilderIds?.map((e) => e) || [];
    message.bugUrlTemplate = object.bugUrlTemplate ?? "";
    message.metadataConfig = (object.metadataConfig !== undefined && object.metadataConfig !== null)
      ? MetadataConfig.fromPartial(object.metadataConfig)
      : undefined;
    return message;
  },
};

function createBaseLink(): Link {
  return { text: "", url: "", alt: "" };
}

export const Link = {
  encode(message: Link, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.text !== "") {
      writer.uint32(10).string(message.text);
    }
    if (message.url !== "") {
      writer.uint32(18).string(message.url);
    }
    if (message.alt !== "") {
      writer.uint32(26).string(message.alt);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Link {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLink() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.text = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.url = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.alt = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Link {
    return {
      text: isSet(object.text) ? globalThis.String(object.text) : "",
      url: isSet(object.url) ? globalThis.String(object.url) : "",
      alt: isSet(object.alt) ? globalThis.String(object.alt) : "",
    };
  },

  toJSON(message: Link): unknown {
    const obj: any = {};
    if (message.text !== "") {
      obj.text = message.text;
    }
    if (message.url !== "") {
      obj.url = message.url;
    }
    if (message.alt !== "") {
      obj.alt = message.alt;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Link>, I>>(base?: I): Link {
    return Link.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Link>, I>>(object: I): Link {
    const message = createBaseLink() as any;
    message.text = object.text ?? "";
    message.url = object.url ?? "";
    message.alt = object.alt ?? "";
    return message;
  },
};

function createBaseOncall(): Oncall {
  return { name: "", url: "", showPrimarySecondaryLabels: false };
}

export const Oncall = {
  encode(message: Oncall, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.url !== "") {
      writer.uint32(18).string(message.url);
    }
    if (message.showPrimarySecondaryLabels === true) {
      writer.uint32(24).bool(message.showPrimarySecondaryLabels);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Oncall {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseOncall() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.url = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.showPrimarySecondaryLabels = reader.bool();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Oncall {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      url: isSet(object.url) ? globalThis.String(object.url) : "",
      showPrimarySecondaryLabels: isSet(object.showPrimarySecondaryLabels)
        ? globalThis.Boolean(object.showPrimarySecondaryLabels)
        : false,
    };
  },

  toJSON(message: Oncall): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.url !== "") {
      obj.url = message.url;
    }
    if (message.showPrimarySecondaryLabels === true) {
      obj.showPrimarySecondaryLabels = message.showPrimarySecondaryLabels;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Oncall>, I>>(base?: I): Oncall {
    return Oncall.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Oncall>, I>>(object: I): Oncall {
    const message = createBaseOncall() as any;
    message.name = object.name ?? "";
    message.url = object.url ?? "";
    message.showPrimarySecondaryLabels = object.showPrimarySecondaryLabels ?? false;
    return message;
  },
};

function createBaseLinkGroup(): LinkGroup {
  return { name: "", links: [] };
}

export const LinkGroup = {
  encode(message: LinkGroup, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    for (const v of message.links) {
      Link.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): LinkGroup {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLinkGroup() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.links.push(Link.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): LinkGroup {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      links: globalThis.Array.isArray(object?.links) ? object.links.map((e: any) => Link.fromJSON(e)) : [],
    };
  },

  toJSON(message: LinkGroup): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.links?.length) {
      obj.links = message.links.map((e) => Link.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<LinkGroup>, I>>(base?: I): LinkGroup {
    return LinkGroup.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<LinkGroup>, I>>(object: I): LinkGroup {
    const message = createBaseLinkGroup() as any;
    message.name = object.name ?? "";
    message.links = object.links?.map((e) => Link.fromPartial(e)) || [];
    return message;
  },
};

function createBaseConsoleSummaryGroup(): ConsoleSummaryGroup {
  return { title: undefined, consoleIds: [] };
}

export const ConsoleSummaryGroup = {
  encode(message: ConsoleSummaryGroup, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.title !== undefined) {
      Link.encode(message.title, writer.uint32(10).fork()).ldelim();
    }
    for (const v of message.consoleIds) {
      writer.uint32(18).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ConsoleSummaryGroup {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConsoleSummaryGroup() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.title = Link.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.consoleIds.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ConsoleSummaryGroup {
    return {
      title: isSet(object.title) ? Link.fromJSON(object.title) : undefined,
      consoleIds: globalThis.Array.isArray(object?.consoleIds)
        ? object.consoleIds.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: ConsoleSummaryGroup): unknown {
    const obj: any = {};
    if (message.title !== undefined) {
      obj.title = Link.toJSON(message.title);
    }
    if (message.consoleIds?.length) {
      obj.consoleIds = message.consoleIds;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ConsoleSummaryGroup>, I>>(base?: I): ConsoleSummaryGroup {
    return ConsoleSummaryGroup.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ConsoleSummaryGroup>, I>>(object: I): ConsoleSummaryGroup {
    const message = createBaseConsoleSummaryGroup() as any;
    message.title = (object.title !== undefined && object.title !== null) ? Link.fromPartial(object.title) : undefined;
    message.consoleIds = object.consoleIds?.map((e) => e) || [];
    return message;
  },
};

function createBaseHeader(): Header {
  return { oncalls: [], links: [], consoleGroups: [], treeStatusHost: "", id: "" };
}

export const Header = {
  encode(message: Header, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.oncalls) {
      Oncall.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    for (const v of message.links) {
      LinkGroup.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.consoleGroups) {
      ConsoleSummaryGroup.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    if (message.treeStatusHost !== "") {
      writer.uint32(34).string(message.treeStatusHost);
    }
    if (message.id !== "") {
      writer.uint32(42).string(message.id);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Header {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseHeader() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.oncalls.push(Oncall.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.links.push(LinkGroup.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.consoleGroups.push(ConsoleSummaryGroup.decode(reader, reader.uint32()));
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.treeStatusHost = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.id = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Header {
    return {
      oncalls: globalThis.Array.isArray(object?.oncalls) ? object.oncalls.map((e: any) => Oncall.fromJSON(e)) : [],
      links: globalThis.Array.isArray(object?.links) ? object.links.map((e: any) => LinkGroup.fromJSON(e)) : [],
      consoleGroups: globalThis.Array.isArray(object?.consoleGroups)
        ? object.consoleGroups.map((e: any) => ConsoleSummaryGroup.fromJSON(e))
        : [],
      treeStatusHost: isSet(object.treeStatusHost) ? globalThis.String(object.treeStatusHost) : "",
      id: isSet(object.id) ? globalThis.String(object.id) : "",
    };
  },

  toJSON(message: Header): unknown {
    const obj: any = {};
    if (message.oncalls?.length) {
      obj.oncalls = message.oncalls.map((e) => Oncall.toJSON(e));
    }
    if (message.links?.length) {
      obj.links = message.links.map((e) => LinkGroup.toJSON(e));
    }
    if (message.consoleGroups?.length) {
      obj.consoleGroups = message.consoleGroups.map((e) => ConsoleSummaryGroup.toJSON(e));
    }
    if (message.treeStatusHost !== "") {
      obj.treeStatusHost = message.treeStatusHost;
    }
    if (message.id !== "") {
      obj.id = message.id;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Header>, I>>(base?: I): Header {
    return Header.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Header>, I>>(object: I): Header {
    const message = createBaseHeader() as any;
    message.oncalls = object.oncalls?.map((e) => Oncall.fromPartial(e)) || [];
    message.links = object.links?.map((e) => LinkGroup.fromPartial(e)) || [];
    message.consoleGroups = object.consoleGroups?.map((e) => ConsoleSummaryGroup.fromPartial(e)) || [];
    message.treeStatusHost = object.treeStatusHost ?? "";
    message.id = object.id ?? "";
    return message;
  },
};

function createBaseConsole(): Console {
  return {
    id: "",
    name: "",
    repoUrl: "",
    refs: [],
    excludeRef: "",
    manifestName: "",
    builders: [],
    faviconUrl: "",
    header: undefined,
    headerId: "",
    includeExperimentalBuilds: false,
    builderViewOnly: false,
    defaultCommitLimit: 0,
    defaultExpand: false,
    externalProject: "",
    externalId: "",
    realm: "",
  };
}

export const Console = {
  encode(message: Console, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.name !== "") {
      writer.uint32(18).string(message.name);
    }
    if (message.repoUrl !== "") {
      writer.uint32(26).string(message.repoUrl);
    }
    for (const v of message.refs) {
      writer.uint32(114).string(v!);
    }
    if (message.excludeRef !== "") {
      writer.uint32(106).string(message.excludeRef);
    }
    if (message.manifestName !== "") {
      writer.uint32(42).string(message.manifestName);
    }
    for (const v of message.builders) {
      Builder.encode(v!, writer.uint32(50).fork()).ldelim();
    }
    if (message.faviconUrl !== "") {
      writer.uint32(58).string(message.faviconUrl);
    }
    if (message.header !== undefined) {
      Header.encode(message.header, writer.uint32(74).fork()).ldelim();
    }
    if (message.headerId !== "") {
      writer.uint32(82).string(message.headerId);
    }
    if (message.includeExperimentalBuilds === true) {
      writer.uint32(88).bool(message.includeExperimentalBuilds);
    }
    if (message.builderViewOnly === true) {
      writer.uint32(96).bool(message.builderViewOnly);
    }
    if (message.defaultCommitLimit !== 0) {
      writer.uint32(120).int32(message.defaultCommitLimit);
    }
    if (message.defaultExpand === true) {
      writer.uint32(128).bool(message.defaultExpand);
    }
    if (message.externalProject !== "") {
      writer.uint32(138).string(message.externalProject);
    }
    if (message.externalId !== "") {
      writer.uint32(146).string(message.externalId);
    }
    if (message.realm !== "") {
      writer.uint32(154).string(message.realm);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Console {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConsole() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.id = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.name = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.repoUrl = reader.string();
          continue;
        case 14:
          if (tag !== 114) {
            break;
          }

          message.refs.push(reader.string());
          continue;
        case 13:
          if (tag !== 106) {
            break;
          }

          message.excludeRef = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.manifestName = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.builders.push(Builder.decode(reader, reader.uint32()));
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.faviconUrl = reader.string();
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.header = Header.decode(reader, reader.uint32());
          continue;
        case 10:
          if (tag !== 82) {
            break;
          }

          message.headerId = reader.string();
          continue;
        case 11:
          if (tag !== 88) {
            break;
          }

          message.includeExperimentalBuilds = reader.bool();
          continue;
        case 12:
          if (tag !== 96) {
            break;
          }

          message.builderViewOnly = reader.bool();
          continue;
        case 15:
          if (tag !== 120) {
            break;
          }

          message.defaultCommitLimit = reader.int32();
          continue;
        case 16:
          if (tag !== 128) {
            break;
          }

          message.defaultExpand = reader.bool();
          continue;
        case 17:
          if (tag !== 138) {
            break;
          }

          message.externalProject = reader.string();
          continue;
        case 18:
          if (tag !== 146) {
            break;
          }

          message.externalId = reader.string();
          continue;
        case 19:
          if (tag !== 154) {
            break;
          }

          message.realm = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Console {
    return {
      id: isSet(object.id) ? globalThis.String(object.id) : "",
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      repoUrl: isSet(object.repoUrl) ? globalThis.String(object.repoUrl) : "",
      refs: globalThis.Array.isArray(object?.refs) ? object.refs.map((e: any) => globalThis.String(e)) : [],
      excludeRef: isSet(object.excludeRef) ? globalThis.String(object.excludeRef) : "",
      manifestName: isSet(object.manifestName) ? globalThis.String(object.manifestName) : "",
      builders: globalThis.Array.isArray(object?.builders) ? object.builders.map((e: any) => Builder.fromJSON(e)) : [],
      faviconUrl: isSet(object.faviconUrl) ? globalThis.String(object.faviconUrl) : "",
      header: isSet(object.header) ? Header.fromJSON(object.header) : undefined,
      headerId: isSet(object.headerId) ? globalThis.String(object.headerId) : "",
      includeExperimentalBuilds: isSet(object.includeExperimentalBuilds)
        ? globalThis.Boolean(object.includeExperimentalBuilds)
        : false,
      builderViewOnly: isSet(object.builderViewOnly) ? globalThis.Boolean(object.builderViewOnly) : false,
      defaultCommitLimit: isSet(object.defaultCommitLimit) ? globalThis.Number(object.defaultCommitLimit) : 0,
      defaultExpand: isSet(object.defaultExpand) ? globalThis.Boolean(object.defaultExpand) : false,
      externalProject: isSet(object.externalProject) ? globalThis.String(object.externalProject) : "",
      externalId: isSet(object.externalId) ? globalThis.String(object.externalId) : "",
      realm: isSet(object.realm) ? globalThis.String(object.realm) : "",
    };
  },

  toJSON(message: Console): unknown {
    const obj: any = {};
    if (message.id !== "") {
      obj.id = message.id;
    }
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.repoUrl !== "") {
      obj.repoUrl = message.repoUrl;
    }
    if (message.refs?.length) {
      obj.refs = message.refs;
    }
    if (message.excludeRef !== "") {
      obj.excludeRef = message.excludeRef;
    }
    if (message.manifestName !== "") {
      obj.manifestName = message.manifestName;
    }
    if (message.builders?.length) {
      obj.builders = message.builders.map((e) => Builder.toJSON(e));
    }
    if (message.faviconUrl !== "") {
      obj.faviconUrl = message.faviconUrl;
    }
    if (message.header !== undefined) {
      obj.header = Header.toJSON(message.header);
    }
    if (message.headerId !== "") {
      obj.headerId = message.headerId;
    }
    if (message.includeExperimentalBuilds === true) {
      obj.includeExperimentalBuilds = message.includeExperimentalBuilds;
    }
    if (message.builderViewOnly === true) {
      obj.builderViewOnly = message.builderViewOnly;
    }
    if (message.defaultCommitLimit !== 0) {
      obj.defaultCommitLimit = Math.round(message.defaultCommitLimit);
    }
    if (message.defaultExpand === true) {
      obj.defaultExpand = message.defaultExpand;
    }
    if (message.externalProject !== "") {
      obj.externalProject = message.externalProject;
    }
    if (message.externalId !== "") {
      obj.externalId = message.externalId;
    }
    if (message.realm !== "") {
      obj.realm = message.realm;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Console>, I>>(base?: I): Console {
    return Console.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Console>, I>>(object: I): Console {
    const message = createBaseConsole() as any;
    message.id = object.id ?? "";
    message.name = object.name ?? "";
    message.repoUrl = object.repoUrl ?? "";
    message.refs = object.refs?.map((e) => e) || [];
    message.excludeRef = object.excludeRef ?? "";
    message.manifestName = object.manifestName ?? "";
    message.builders = object.builders?.map((e) => Builder.fromPartial(e)) || [];
    message.faviconUrl = object.faviconUrl ?? "";
    message.header = (object.header !== undefined && object.header !== null)
      ? Header.fromPartial(object.header)
      : undefined;
    message.headerId = object.headerId ?? "";
    message.includeExperimentalBuilds = object.includeExperimentalBuilds ?? false;
    message.builderViewOnly = object.builderViewOnly ?? false;
    message.defaultCommitLimit = object.defaultCommitLimit ?? 0;
    message.defaultExpand = object.defaultExpand ?? false;
    message.externalProject = object.externalProject ?? "";
    message.externalId = object.externalId ?? "";
    message.realm = object.realm ?? "";
    return message;
  },
};

function createBaseBuilder(): Builder {
  return { name: "", id: undefined, category: "", shortName: "" };
}

export const Builder = {
  encode(message: Builder, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.id !== undefined) {
      BuilderID.encode(message.id, writer.uint32(34).fork()).ldelim();
    }
    if (message.category !== "") {
      writer.uint32(18).string(message.category);
    }
    if (message.shortName !== "") {
      writer.uint32(26).string(message.shortName);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Builder {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilder() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.id = BuilderID.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.category = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.shortName = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Builder {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      id: isSet(object.id) ? BuilderID.fromJSON(object.id) : undefined,
      category: isSet(object.category) ? globalThis.String(object.category) : "",
      shortName: isSet(object.shortName) ? globalThis.String(object.shortName) : "",
    };
  },

  toJSON(message: Builder): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.id !== undefined) {
      obj.id = BuilderID.toJSON(message.id);
    }
    if (message.category !== "") {
      obj.category = message.category;
    }
    if (message.shortName !== "") {
      obj.shortName = message.shortName;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Builder>, I>>(base?: I): Builder {
    return Builder.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Builder>, I>>(object: I): Builder {
    const message = createBaseBuilder() as any;
    message.name = object.name ?? "";
    message.id = (object.id !== undefined && object.id !== null) ? BuilderID.fromPartial(object.id) : undefined;
    message.category = object.category ?? "";
    message.shortName = object.shortName ?? "";
    return message;
  },
};

function createBaseMetadataConfig(): MetadataConfig {
  return { testMetadataProperties: [] };
}

export const MetadataConfig = {
  encode(message: MetadataConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.testMetadataProperties) {
      DisplayRule.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MetadataConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMetadataConfig() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testMetadataProperties.push(DisplayRule.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): MetadataConfig {
    return {
      testMetadataProperties: globalThis.Array.isArray(object?.testMetadataProperties)
        ? object.testMetadataProperties.map((e: any) => DisplayRule.fromJSON(e))
        : [],
    };
  },

  toJSON(message: MetadataConfig): unknown {
    const obj: any = {};
    if (message.testMetadataProperties?.length) {
      obj.testMetadataProperties = message.testMetadataProperties.map((e) => DisplayRule.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<MetadataConfig>, I>>(base?: I): MetadataConfig {
    return MetadataConfig.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<MetadataConfig>, I>>(object: I): MetadataConfig {
    const message = createBaseMetadataConfig() as any;
    message.testMetadataProperties = object.testMetadataProperties?.map((e) => DisplayRule.fromPartial(e)) || [];
    return message;
  },
};

function createBaseDisplayRule(): DisplayRule {
  return { schema: "", displayItems: [] };
}

export const DisplayRule = {
  encode(message: DisplayRule, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.schema !== "") {
      writer.uint32(10).string(message.schema);
    }
    for (const v of message.displayItems) {
      DisplayItem.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DisplayRule {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDisplayRule() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.schema = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.displayItems.push(DisplayItem.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): DisplayRule {
    return {
      schema: isSet(object.schema) ? globalThis.String(object.schema) : "",
      displayItems: globalThis.Array.isArray(object?.displayItems)
        ? object.displayItems.map((e: any) => DisplayItem.fromJSON(e))
        : [],
    };
  },

  toJSON(message: DisplayRule): unknown {
    const obj: any = {};
    if (message.schema !== "") {
      obj.schema = message.schema;
    }
    if (message.displayItems?.length) {
      obj.displayItems = message.displayItems.map((e) => DisplayItem.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<DisplayRule>, I>>(base?: I): DisplayRule {
    return DisplayRule.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<DisplayRule>, I>>(object: I): DisplayRule {
    const message = createBaseDisplayRule() as any;
    message.schema = object.schema ?? "";
    message.displayItems = object.displayItems?.map((e) => DisplayItem.fromPartial(e)) || [];
    return message;
  },
};

function createBaseDisplayItem(): DisplayItem {
  return { displayName: "", path: "" };
}

export const DisplayItem = {
  encode(message: DisplayItem, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.displayName !== "") {
      writer.uint32(10).string(message.displayName);
    }
    if (message.path !== "") {
      writer.uint32(18).string(message.path);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DisplayItem {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDisplayItem() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.displayName = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.path = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): DisplayItem {
    return {
      displayName: isSet(object.displayName) ? globalThis.String(object.displayName) : "",
      path: isSet(object.path) ? globalThis.String(object.path) : "",
    };
  },

  toJSON(message: DisplayItem): unknown {
    const obj: any = {};
    if (message.displayName !== "") {
      obj.displayName = message.displayName;
    }
    if (message.path !== "") {
      obj.path = message.path;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<DisplayItem>, I>>(base?: I): DisplayItem {
    return DisplayItem.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<DisplayItem>, I>>(object: I): DisplayItem {
    const message = createBaseDisplayItem() as any;
    message.displayName = object.displayName ?? "";
    message.path = object.path ?? "";
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

type KeysOfUnion<T> = T extends T ? keyof T : never;
export type Exact<P, I extends P> = P extends Builtin ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & { [K in Exclude<keyof I, KeysOfUnion<P>>]: never };

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}