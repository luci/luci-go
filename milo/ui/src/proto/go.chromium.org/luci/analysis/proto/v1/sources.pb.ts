/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";

export const protobufPackage = "luci.analysis.v1";

/** ChangelistOwner describes the owner of a gerrit changelist. */
export enum ChangelistOwnerKind {
  /** CHANGELIST_OWNER_UNSPECIFIED - The changelist owner is not known. */
  CHANGELIST_OWNER_UNSPECIFIED = 0,
  /** HUMAN - The changelist is owned by a human. */
  HUMAN = 1,
  /**
   * AUTOMATION - The changelist is owned by automation. (E.g. autoroller or
   * automatic uprev process.)
   */
  AUTOMATION = 2,
}

export function changelistOwnerKindFromJSON(object: any): ChangelistOwnerKind {
  switch (object) {
    case 0:
    case "CHANGELIST_OWNER_UNSPECIFIED":
      return ChangelistOwnerKind.CHANGELIST_OWNER_UNSPECIFIED;
    case 1:
    case "HUMAN":
      return ChangelistOwnerKind.HUMAN;
    case 2:
    case "AUTOMATION":
      return ChangelistOwnerKind.AUTOMATION;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum ChangelistOwnerKind");
  }
}

export function changelistOwnerKindToJSON(object: ChangelistOwnerKind): string {
  switch (object) {
    case ChangelistOwnerKind.CHANGELIST_OWNER_UNSPECIFIED:
      return "CHANGELIST_OWNER_UNSPECIFIED";
    case ChangelistOwnerKind.HUMAN:
      return "HUMAN";
    case ChangelistOwnerKind.AUTOMATION:
      return "AUTOMATION";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum ChangelistOwnerKind");
  }
}

/** Specifies the source code that was tested. */
export interface Sources {
  /**
   * The base version of code sources checked out. Mandatory.
   * If necessary, we could add support for non-gitiles sources here in
   * future, using a oneof statement. E.g.
   * oneof system {
   *    GitilesCommit gitiles_commit = 1;
   *    SubversionRevision svn_revision = 4;
   *    ...
   * }
   */
  readonly gitilesCommit:
    | GitilesCommit
    | undefined;
  /**
   * The changelist(s) which were applied upon the base version of sources
   * checked out. E.g. in commit queue tryjobs.
   *
   * At most 10 changelist(s) may be specified here. If there
   * are more, only include the first 10 and set is_dirty.
   */
  readonly changelists: readonly GerritChange[];
  /**
   * Whether there were any changes made to the sources, not described above.
   * For example, a version of a dependency was upgraded before testing (e.g.
   * in an autoroller recipe).
   *
   * Cherry-picking a changelist on top of the base checkout is not considered
   * making the sources dirty as it is reported separately above.
   */
  readonly isDirty: boolean;
}

/**
 * GitilesCommit specifies the position of the gitiles commit an invocation
 * ran against, in a repository's commit log. More specifically, a ref's commit
 * log.
 *
 * It also specifies the host/project/ref combination that the commit
 * exists in, to provide context.
 */
export interface GitilesCommit {
  /**
   * The identity of the gitiles host, e.g. "chromium.googlesource.com".
   * Mandatory.
   */
  readonly host: string;
  /** Repository name on the host, e.g. "chromium/src". Mandatory. */
  readonly project: string;
  /**
   * Commit ref, e.g. "refs/heads/main" from which the commit was fetched.
   * Not the branch name, use "refs/heads/branch"
   * Mandatory.
   */
  readonly ref: string;
  /** Commit SHA-1, as 40 lowercase hexadecimal characters. Mandatory. */
  readonly commitHash: string;
  /**
   * Defines a total order of commits on the ref.
   * A positive, monotonically increasing integer. The recommended
   * way of obtaining this is by using the goto.google.com/git-numberer
   * Gerrit plugin. Other solutions can be used as well, so long
   * as the same scheme is used consistently for a ref.
   * Mandatory.
   */
  readonly position: string;
}

/** A Gerrit patchset. */
export interface GerritChange {
  /** Gerrit hostname, e.g. "chromium-review.googlesource.com". */
  readonly host: string;
  /** Gerrit project, e.g. "chromium/src". */
  readonly project: string;
  /** Change number, e.g. 12345. */
  readonly change: string;
  /** Patch set number, e.g. 1. */
  readonly patchset: string;
}

/** A gerrit changelist. */
export interface Changelist {
  /** Gerrit hostname, e.g. "chromium-review.googlesource.com". */
  readonly host: string;
  /** Change number, e.g. 12345. */
  readonly change: string;
  /** Patch set number, e.g. 1. */
  readonly patchset: number;
  /** The kind of owner of the changelist. */
  readonly ownerKind: ChangelistOwnerKind;
}

function createBaseSources(): Sources {
  return { gitilesCommit: undefined, changelists: [], isDirty: false };
}

export const Sources = {
  encode(message: Sources, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.gitilesCommit !== undefined) {
      GitilesCommit.encode(message.gitilesCommit, writer.uint32(10).fork()).ldelim();
    }
    for (const v of message.changelists) {
      GerritChange.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    if (message.isDirty === true) {
      writer.uint32(24).bool(message.isDirty);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Sources {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSources() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.gitilesCommit = GitilesCommit.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.changelists.push(GerritChange.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.isDirty = reader.bool();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Sources {
    return {
      gitilesCommit: isSet(object.gitilesCommit) ? GitilesCommit.fromJSON(object.gitilesCommit) : undefined,
      changelists: globalThis.Array.isArray(object?.changelists)
        ? object.changelists.map((e: any) => GerritChange.fromJSON(e))
        : [],
      isDirty: isSet(object.isDirty) ? globalThis.Boolean(object.isDirty) : false,
    };
  },

  toJSON(message: Sources): unknown {
    const obj: any = {};
    if (message.gitilesCommit !== undefined) {
      obj.gitilesCommit = GitilesCommit.toJSON(message.gitilesCommit);
    }
    if (message.changelists?.length) {
      obj.changelists = message.changelists.map((e) => GerritChange.toJSON(e));
    }
    if (message.isDirty === true) {
      obj.isDirty = message.isDirty;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Sources>, I>>(base?: I): Sources {
    return Sources.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Sources>, I>>(object: I): Sources {
    const message = createBaseSources() as any;
    message.gitilesCommit = (object.gitilesCommit !== undefined && object.gitilesCommit !== null)
      ? GitilesCommit.fromPartial(object.gitilesCommit)
      : undefined;
    message.changelists = object.changelists?.map((e) => GerritChange.fromPartial(e)) || [];
    message.isDirty = object.isDirty ?? false;
    return message;
  },
};

function createBaseGitilesCommit(): GitilesCommit {
  return { host: "", project: "", ref: "", commitHash: "", position: "0" };
}

export const GitilesCommit = {
  encode(message: GitilesCommit, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.host !== "") {
      writer.uint32(10).string(message.host);
    }
    if (message.project !== "") {
      writer.uint32(18).string(message.project);
    }
    if (message.ref !== "") {
      writer.uint32(26).string(message.ref);
    }
    if (message.commitHash !== "") {
      writer.uint32(34).string(message.commitHash);
    }
    if (message.position !== "0") {
      writer.uint32(40).int64(message.position);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GitilesCommit {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGitilesCommit() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.host = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.project = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.ref = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.commitHash = reader.string();
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.position = longToString(reader.int64() as Long);
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): GitilesCommit {
    return {
      host: isSet(object.host) ? globalThis.String(object.host) : "",
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      ref: isSet(object.ref) ? globalThis.String(object.ref) : "",
      commitHash: isSet(object.commitHash) ? globalThis.String(object.commitHash) : "",
      position: isSet(object.position) ? globalThis.String(object.position) : "0",
    };
  },

  toJSON(message: GitilesCommit): unknown {
    const obj: any = {};
    if (message.host !== "") {
      obj.host = message.host;
    }
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.ref !== "") {
      obj.ref = message.ref;
    }
    if (message.commitHash !== "") {
      obj.commitHash = message.commitHash;
    }
    if (message.position !== "0") {
      obj.position = message.position;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GitilesCommit>, I>>(base?: I): GitilesCommit {
    return GitilesCommit.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GitilesCommit>, I>>(object: I): GitilesCommit {
    const message = createBaseGitilesCommit() as any;
    message.host = object.host ?? "";
    message.project = object.project ?? "";
    message.ref = object.ref ?? "";
    message.commitHash = object.commitHash ?? "";
    message.position = object.position ?? "0";
    return message;
  },
};

function createBaseGerritChange(): GerritChange {
  return { host: "", project: "", change: "0", patchset: "0" };
}

export const GerritChange = {
  encode(message: GerritChange, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.host !== "") {
      writer.uint32(10).string(message.host);
    }
    if (message.project !== "") {
      writer.uint32(42).string(message.project);
    }
    if (message.change !== "0") {
      writer.uint32(16).int64(message.change);
    }
    if (message.patchset !== "0") {
      writer.uint32(24).int64(message.patchset);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GerritChange {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGerritChange() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.host = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.project = reader.string();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.change = longToString(reader.int64() as Long);
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.patchset = longToString(reader.int64() as Long);
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): GerritChange {
    return {
      host: isSet(object.host) ? globalThis.String(object.host) : "",
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      change: isSet(object.change) ? globalThis.String(object.change) : "0",
      patchset: isSet(object.patchset) ? globalThis.String(object.patchset) : "0",
    };
  },

  toJSON(message: GerritChange): unknown {
    const obj: any = {};
    if (message.host !== "") {
      obj.host = message.host;
    }
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.change !== "0") {
      obj.change = message.change;
    }
    if (message.patchset !== "0") {
      obj.patchset = message.patchset;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GerritChange>, I>>(base?: I): GerritChange {
    return GerritChange.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GerritChange>, I>>(object: I): GerritChange {
    const message = createBaseGerritChange() as any;
    message.host = object.host ?? "";
    message.project = object.project ?? "";
    message.change = object.change ?? "0";
    message.patchset = object.patchset ?? "0";
    return message;
  },
};

function createBaseChangelist(): Changelist {
  return { host: "", change: "0", patchset: 0, ownerKind: 0 };
}

export const Changelist = {
  encode(message: Changelist, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.host !== "") {
      writer.uint32(10).string(message.host);
    }
    if (message.change !== "0") {
      writer.uint32(16).int64(message.change);
    }
    if (message.patchset !== 0) {
      writer.uint32(24).int32(message.patchset);
    }
    if (message.ownerKind !== 0) {
      writer.uint32(32).int32(message.ownerKind);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Changelist {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseChangelist() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.host = reader.string();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.change = longToString(reader.int64() as Long);
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.patchset = reader.int32();
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.ownerKind = reader.int32() as any;
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Changelist {
    return {
      host: isSet(object.host) ? globalThis.String(object.host) : "",
      change: isSet(object.change) ? globalThis.String(object.change) : "0",
      patchset: isSet(object.patchset) ? globalThis.Number(object.patchset) : 0,
      ownerKind: isSet(object.ownerKind) ? changelistOwnerKindFromJSON(object.ownerKind) : 0,
    };
  },

  toJSON(message: Changelist): unknown {
    const obj: any = {};
    if (message.host !== "") {
      obj.host = message.host;
    }
    if (message.change !== "0") {
      obj.change = message.change;
    }
    if (message.patchset !== 0) {
      obj.patchset = Math.round(message.patchset);
    }
    if (message.ownerKind !== 0) {
      obj.ownerKind = changelistOwnerKindToJSON(message.ownerKind);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Changelist>, I>>(base?: I): Changelist {
    return Changelist.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Changelist>, I>>(object: I): Changelist {
    const message = createBaseChangelist() as any;
    message.host = object.host ?? "";
    message.change = object.change ?? "0";
    message.patchset = object.patchset ?? 0;
    message.ownerKind = object.ownerKind ?? 0;
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

function longToString(long: Long) {
  return long.toString();
}

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any;
  _m0.configure();
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
