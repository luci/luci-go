/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";

export const protobufPackage = "git";

/**
 * Commit is a single parsed commit as represented in a git log or git show
 * expression.
 */
export interface Commit {
  /** The hex sha1 of the commit. */
  readonly id: string;
  /** The hex sha1 of the tree for this commit. */
  readonly tree: string;
  /** The hex sha1's of each of this commits' parents. */
  readonly parents: readonly string[];
  readonly author: Commit_User | undefined;
  readonly committer:
    | Commit_User
    | undefined;
  /** This is the entire unaltered message body. */
  readonly message: string;
  readonly treeDiff: readonly Commit_TreeDiff[];
}

/**
 * User represents the (name, email, timestamp) Commit header for author and/or
 * commtter.
 */
export interface Commit_User {
  readonly name: string;
  readonly email: string;
  readonly time: string | undefined;
}

/**
 * Each TreeDiff represents a single file that's changed between this commit
 * and the "previous" commit, where "previous" depends on the context of how
 * this Commit object was produced (i.e. the specific `git log` invocation, or
 * similar command).
 *
 * Note that these are an artifact of the `git log` expression, not of the
 * commit itself (since git log has different ways that it could sort the
 * commits in the log, and thus different ways it could calculate these
 * diffs). In particular, you should avoid caching the TreeDiff data using
 * only the Commit.id as the key.
 *
 * The old_* fields correspond to the matching file in the previous commit (in
 * the case of COPY/DELETE/MODIFY/RENAME), telling its blob hash, file mode
 * and path name.
 *
 * The new_* fields correspond to the matching file in this commit (in the
 * case of ADD/COPY/MODIFY/RENAME), telling its blob hash, file mode and path
 * name.
 */
export interface Commit_TreeDiff {
  /** How this file changed. */
  readonly type: Commit_TreeDiff_ChangeType;
  readonly oldId: string;
  readonly oldMode: number;
  readonly oldPath: string;
  readonly newId: string;
  readonly newMode: number;
  readonly newPath: string;
}

export enum Commit_TreeDiff_ChangeType {
  ADD = 0,
  COPY = 1,
  DELETE = 2,
  MODIFY = 3,
  RENAME = 4,
}

export function commit_TreeDiff_ChangeTypeFromJSON(object: any): Commit_TreeDiff_ChangeType {
  switch (object) {
    case 0:
    case "ADD":
      return Commit_TreeDiff_ChangeType.ADD;
    case 1:
    case "COPY":
      return Commit_TreeDiff_ChangeType.COPY;
    case 2:
    case "DELETE":
      return Commit_TreeDiff_ChangeType.DELETE;
    case 3:
    case "MODIFY":
      return Commit_TreeDiff_ChangeType.MODIFY;
    case 4:
    case "RENAME":
      return Commit_TreeDiff_ChangeType.RENAME;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Commit_TreeDiff_ChangeType");
  }
}

export function commit_TreeDiff_ChangeTypeToJSON(object: Commit_TreeDiff_ChangeType): string {
  switch (object) {
    case Commit_TreeDiff_ChangeType.ADD:
      return "ADD";
    case Commit_TreeDiff_ChangeType.COPY:
      return "COPY";
    case Commit_TreeDiff_ChangeType.DELETE:
      return "DELETE";
    case Commit_TreeDiff_ChangeType.MODIFY:
      return "MODIFY";
    case Commit_TreeDiff_ChangeType.RENAME:
      return "RENAME";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Commit_TreeDiff_ChangeType");
  }
}

/** File is a single file as represented in a git tree. */
export interface File {
  /** ID is sha1 hash of the file contents */
  readonly id: string;
  /** Path is the path to file, without leading "/". */
  readonly path: string;
  /** Mode is file mode, e.g. 0100644 (octal, often shows up 33188 in decimal). */
  readonly mode: number;
  /** Type is the file type. */
  readonly type: File_Type;
}

export enum File_Type {
  UNKNOWN = 0,
  TREE = 1,
  BLOB = 2,
}

export function file_TypeFromJSON(object: any): File_Type {
  switch (object) {
    case 0:
    case "UNKNOWN":
      return File_Type.UNKNOWN;
    case 1:
    case "TREE":
      return File_Type.TREE;
    case 2:
    case "BLOB":
      return File_Type.BLOB;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum File_Type");
  }
}

export function file_TypeToJSON(object: File_Type): string {
  switch (object) {
    case File_Type.UNKNOWN:
      return "UNKNOWN";
    case File_Type.TREE:
      return "TREE";
    case File_Type.BLOB:
      return "BLOB";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum File_Type");
  }
}

function createBaseCommit(): Commit {
  return { id: "", tree: "", parents: [], author: undefined, committer: undefined, message: "", treeDiff: [] };
}

export const Commit = {
  encode(message: Commit, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.tree !== "") {
      writer.uint32(18).string(message.tree);
    }
    for (const v of message.parents) {
      writer.uint32(26).string(v!);
    }
    if (message.author !== undefined) {
      Commit_User.encode(message.author, writer.uint32(34).fork()).ldelim();
    }
    if (message.committer !== undefined) {
      Commit_User.encode(message.committer, writer.uint32(42).fork()).ldelim();
    }
    if (message.message !== "") {
      writer.uint32(50).string(message.message);
    }
    for (const v of message.treeDiff) {
      Commit_TreeDiff.encode(v!, writer.uint32(58).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Commit {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseCommit() as any;
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

          message.tree = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.parents.push(reader.string());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.author = Commit_User.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.committer = Commit_User.decode(reader, reader.uint32());
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.message = reader.string();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.treeDiff.push(Commit_TreeDiff.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Commit {
    return {
      id: isSet(object.id) ? globalThis.String(object.id) : "",
      tree: isSet(object.tree) ? globalThis.String(object.tree) : "",
      parents: globalThis.Array.isArray(object?.parents) ? object.parents.map((e: any) => globalThis.String(e)) : [],
      author: isSet(object.author) ? Commit_User.fromJSON(object.author) : undefined,
      committer: isSet(object.committer) ? Commit_User.fromJSON(object.committer) : undefined,
      message: isSet(object.message) ? globalThis.String(object.message) : "",
      treeDiff: globalThis.Array.isArray(object?.treeDiff)
        ? object.treeDiff.map((e: any) => Commit_TreeDiff.fromJSON(e))
        : [],
    };
  },

  toJSON(message: Commit): unknown {
    const obj: any = {};
    if (message.id !== "") {
      obj.id = message.id;
    }
    if (message.tree !== "") {
      obj.tree = message.tree;
    }
    if (message.parents?.length) {
      obj.parents = message.parents;
    }
    if (message.author !== undefined) {
      obj.author = Commit_User.toJSON(message.author);
    }
    if (message.committer !== undefined) {
      obj.committer = Commit_User.toJSON(message.committer);
    }
    if (message.message !== "") {
      obj.message = message.message;
    }
    if (message.treeDiff?.length) {
      obj.treeDiff = message.treeDiff.map((e) => Commit_TreeDiff.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Commit>, I>>(base?: I): Commit {
    return Commit.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Commit>, I>>(object: I): Commit {
    const message = createBaseCommit() as any;
    message.id = object.id ?? "";
    message.tree = object.tree ?? "";
    message.parents = object.parents?.map((e) => e) || [];
    message.author = (object.author !== undefined && object.author !== null)
      ? Commit_User.fromPartial(object.author)
      : undefined;
    message.committer = (object.committer !== undefined && object.committer !== null)
      ? Commit_User.fromPartial(object.committer)
      : undefined;
    message.message = object.message ?? "";
    message.treeDiff = object.treeDiff?.map((e) => Commit_TreeDiff.fromPartial(e)) || [];
    return message;
  },
};

function createBaseCommit_User(): Commit_User {
  return { name: "", email: "", time: undefined };
}

export const Commit_User = {
  encode(message: Commit_User, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.email !== "") {
      writer.uint32(18).string(message.email);
    }
    if (message.time !== undefined) {
      Timestamp.encode(toTimestamp(message.time), writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Commit_User {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseCommit_User() as any;
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

          message.email = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.time = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Commit_User {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      email: isSet(object.email) ? globalThis.String(object.email) : "",
      time: isSet(object.time) ? globalThis.String(object.time) : undefined,
    };
  },

  toJSON(message: Commit_User): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.email !== "") {
      obj.email = message.email;
    }
    if (message.time !== undefined) {
      obj.time = message.time;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Commit_User>, I>>(base?: I): Commit_User {
    return Commit_User.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Commit_User>, I>>(object: I): Commit_User {
    const message = createBaseCommit_User() as any;
    message.name = object.name ?? "";
    message.email = object.email ?? "";
    message.time = object.time ?? undefined;
    return message;
  },
};

function createBaseCommit_TreeDiff(): Commit_TreeDiff {
  return { type: 0, oldId: "", oldMode: 0, oldPath: "", newId: "", newMode: 0, newPath: "" };
}

export const Commit_TreeDiff = {
  encode(message: Commit_TreeDiff, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.type !== 0) {
      writer.uint32(8).int32(message.type);
    }
    if (message.oldId !== "") {
      writer.uint32(18).string(message.oldId);
    }
    if (message.oldMode !== 0) {
      writer.uint32(24).uint32(message.oldMode);
    }
    if (message.oldPath !== "") {
      writer.uint32(34).string(message.oldPath);
    }
    if (message.newId !== "") {
      writer.uint32(42).string(message.newId);
    }
    if (message.newMode !== 0) {
      writer.uint32(48).uint32(message.newMode);
    }
    if (message.newPath !== "") {
      writer.uint32(58).string(message.newPath);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Commit_TreeDiff {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseCommit_TreeDiff() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.type = reader.int32() as any;
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.oldId = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.oldMode = reader.uint32();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.oldPath = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.newId = reader.string();
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.newMode = reader.uint32();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.newPath = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Commit_TreeDiff {
    return {
      type: isSet(object.type) ? commit_TreeDiff_ChangeTypeFromJSON(object.type) : 0,
      oldId: isSet(object.oldId) ? globalThis.String(object.oldId) : "",
      oldMode: isSet(object.oldMode) ? globalThis.Number(object.oldMode) : 0,
      oldPath: isSet(object.oldPath) ? globalThis.String(object.oldPath) : "",
      newId: isSet(object.newId) ? globalThis.String(object.newId) : "",
      newMode: isSet(object.newMode) ? globalThis.Number(object.newMode) : 0,
      newPath: isSet(object.newPath) ? globalThis.String(object.newPath) : "",
    };
  },

  toJSON(message: Commit_TreeDiff): unknown {
    const obj: any = {};
    if (message.type !== 0) {
      obj.type = commit_TreeDiff_ChangeTypeToJSON(message.type);
    }
    if (message.oldId !== "") {
      obj.oldId = message.oldId;
    }
    if (message.oldMode !== 0) {
      obj.oldMode = Math.round(message.oldMode);
    }
    if (message.oldPath !== "") {
      obj.oldPath = message.oldPath;
    }
    if (message.newId !== "") {
      obj.newId = message.newId;
    }
    if (message.newMode !== 0) {
      obj.newMode = Math.round(message.newMode);
    }
    if (message.newPath !== "") {
      obj.newPath = message.newPath;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Commit_TreeDiff>, I>>(base?: I): Commit_TreeDiff {
    return Commit_TreeDiff.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Commit_TreeDiff>, I>>(object: I): Commit_TreeDiff {
    const message = createBaseCommit_TreeDiff() as any;
    message.type = object.type ?? 0;
    message.oldId = object.oldId ?? "";
    message.oldMode = object.oldMode ?? 0;
    message.oldPath = object.oldPath ?? "";
    message.newId = object.newId ?? "";
    message.newMode = object.newMode ?? 0;
    message.newPath = object.newPath ?? "";
    return message;
  },
};

function createBaseFile(): File {
  return { id: "", path: "", mode: 0, type: 0 };
}

export const File = {
  encode(message: File, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    if (message.path !== "") {
      writer.uint32(18).string(message.path);
    }
    if (message.mode !== 0) {
      writer.uint32(24).uint32(message.mode);
    }
    if (message.type !== 0) {
      writer.uint32(32).int32(message.type);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): File {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseFile() as any;
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

          message.path = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.mode = reader.uint32();
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.type = reader.int32() as any;
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): File {
    return {
      id: isSet(object.id) ? globalThis.String(object.id) : "",
      path: isSet(object.path) ? globalThis.String(object.path) : "",
      mode: isSet(object.mode) ? globalThis.Number(object.mode) : 0,
      type: isSet(object.type) ? file_TypeFromJSON(object.type) : 0,
    };
  },

  toJSON(message: File): unknown {
    const obj: any = {};
    if (message.id !== "") {
      obj.id = message.id;
    }
    if (message.path !== "") {
      obj.path = message.path;
    }
    if (message.mode !== 0) {
      obj.mode = Math.round(message.mode);
    }
    if (message.type !== 0) {
      obj.type = file_TypeToJSON(message.type);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<File>, I>>(base?: I): File {
    return File.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<File>, I>>(object: I): File {
    const message = createBaseFile() as any;
    message.id = object.id ?? "";
    message.path = object.path ?? "";
    message.mode = object.mode ?? 0;
    message.type = object.type ?? 0;
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

function toTimestamp(dateStr: string): Timestamp {
  const date = new globalThis.Date(dateStr);
  const seconds = Math.trunc(date.getTime() / 1_000).toString();
  const nanos = (date.getTime() % 1_000) * 1_000_000;
  return { seconds, nanos };
}

function fromTimestamp(t: Timestamp): string {
  let millis = (globalThis.Number(t.seconds) || 0) * 1_000;
  millis += (t.nanos || 0) / 1_000_000;
  return new globalThis.Date(millis).toISOString();
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}