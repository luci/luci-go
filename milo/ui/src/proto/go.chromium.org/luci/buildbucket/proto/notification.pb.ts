// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v2.7.5
//   protoc               v6.31.1
// source: go.chromium.org/luci/buildbucket/proto/notification.proto

/* eslint-disable */
import { BinaryReader, BinaryWriter } from "@bufbuild/protobuf/wire";
import { Build } from "./build.pb";
import { Compression, compressionFromJSON, compressionToJSON } from "./common.pb";

export const protobufPackage = "buildbucket.v2";

/**
 * Configuration for per-build notification. It's usually set by the caller on
 * each ScheduleBuild request.
 */
export interface NotificationConfig {
  /**
   * Target Cloud PubSub topic.
   * Usually has format "projects/{cloud project}/topics/{topic name}".
   *
   * The PubSub message data schema is defined in `PubSubCallBack` in this file.
   *
   * The legacy schema is:
   *     {
   *      'build': ${BuildMessage},
   *      'user_data': ${NotificationConfig.user_data}
   *      'hostname': 'cr-buildbucket.appspot.com',
   *    }
   * where the BuildMessage is
   * https://chromium.googlesource.com/infra/infra.git/+/b3204748243a9e4bf815a7024e921be46e3e1747/appengine/cr-buildbucket/legacy/api_common.py#94
   *
   * Note: The legacy data schema is deprecated. Only a few old users are using
   * it and will be migrated soon.
   *
   * <buildbucket-app-id>@appspot.gserviceaccount.com must have
   * "pubsub.topics.publish" and "pubsub.topics.get" permissions on the topic,
   * where <buildbucket-app-id> is usually "cr-buildbucket."
   */
  readonly pubsubTopic: string;
  /**
   * Will be available in PubSubCallBack.user_data.
   * Max length: 4096.
   */
  readonly userData: Uint8Array;
}

/**
 * BuildsV2PubSub is the "builds_v2" pubsub topic message data schema.
 * Attributes of this pubsub message:
 * - "project"
 * - "bucket"
 * - "builder"
 * - "is_completed" (The value is either "true" or "false" in string.)
 * - "version" (The value is "v2". To help distinguish messages from the old `builds` topic)
 */
export interface BuildsV2PubSub {
  /** Contains all field except large fields */
  readonly build:
    | Build
    | undefined;
  /**
   * A Compressed bytes in proto binary format of buildbucket.v2.Build where
   * it only contains the large build fields - build.input.properties,
   * build.output.properties and build.steps.
   */
  readonly buildLargeFields: Uint8Array;
  /**
   * The compression method the above `build_large_fields` uses. By default, it
   * is ZLIB as this is the most common one and is the built-in lib in many
   * programming languages.
   */
  readonly compression: Compression;
  /**
   * A flag to indicate the build large fields are dropped from the PubSub
   * message. This can happen if the large fields are too large even after
   * compression.
   *
   * If you need those fields, please call GetBuild RPC to get them.
   */
  readonly buildLargeFieldsDropped: boolean;
}

/**
 * PubSubCallBack is the message data schema for the ad-hoc pubsub notification
 * specified per ScheduleBuild request level.
 * Attributes of this pubsub message:
 * - "project"
 * - "bucket"
 * - "builder"
 * - "is_completed" (The value is either "true" or "false" in string.)
 * - "version" (The value is "v2". To help distinguish messages from the old `builds` topic)
 */
export interface PubSubCallBack {
  /** Buildbucket build */
  readonly buildPubsub:
    | BuildsV2PubSub
    | undefined;
  /** User-defined opaque blob specified in NotificationConfig.user_data. */
  readonly userData: Uint8Array;
}

function createBaseNotificationConfig(): NotificationConfig {
  return { pubsubTopic: "", userData: new Uint8Array(0) };
}

export const NotificationConfig: MessageFns<NotificationConfig> = {
  encode(message: NotificationConfig, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.pubsubTopic !== "") {
      writer.uint32(10).string(message.pubsubTopic);
    }
    if (message.userData.length !== 0) {
      writer.uint32(18).bytes(message.userData);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): NotificationConfig {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseNotificationConfig() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.pubsubTopic = reader.string();
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.userData = reader.bytes();
          continue;
        }
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skip(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): NotificationConfig {
    return {
      pubsubTopic: isSet(object.pubsubTopic) ? globalThis.String(object.pubsubTopic) : "",
      userData: isSet(object.userData) ? bytesFromBase64(object.userData) : new Uint8Array(0),
    };
  },

  toJSON(message: NotificationConfig): unknown {
    const obj: any = {};
    if (message.pubsubTopic !== "") {
      obj.pubsubTopic = message.pubsubTopic;
    }
    if (message.userData.length !== 0) {
      obj.userData = base64FromBytes(message.userData);
    }
    return obj;
  },

  create(base?: DeepPartial<NotificationConfig>): NotificationConfig {
    return NotificationConfig.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<NotificationConfig>): NotificationConfig {
    const message = createBaseNotificationConfig() as any;
    message.pubsubTopic = object.pubsubTopic ?? "";
    message.userData = object.userData ?? new Uint8Array(0);
    return message;
  },
};

function createBaseBuildsV2PubSub(): BuildsV2PubSub {
  return { build: undefined, buildLargeFields: new Uint8Array(0), compression: 0, buildLargeFieldsDropped: false };
}

export const BuildsV2PubSub: MessageFns<BuildsV2PubSub> = {
  encode(message: BuildsV2PubSub, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.build !== undefined) {
      Build.encode(message.build, writer.uint32(10).fork()).join();
    }
    if (message.buildLargeFields.length !== 0) {
      writer.uint32(18).bytes(message.buildLargeFields);
    }
    if (message.compression !== 0) {
      writer.uint32(24).int32(message.compression);
    }
    if (message.buildLargeFieldsDropped !== false) {
      writer.uint32(32).bool(message.buildLargeFieldsDropped);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): BuildsV2PubSub {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildsV2PubSub() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.build = Build.decode(reader, reader.uint32());
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.buildLargeFields = reader.bytes();
          continue;
        }
        case 3: {
          if (tag !== 24) {
            break;
          }

          message.compression = reader.int32() as any;
          continue;
        }
        case 4: {
          if (tag !== 32) {
            break;
          }

          message.buildLargeFieldsDropped = reader.bool();
          continue;
        }
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skip(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildsV2PubSub {
    return {
      build: isSet(object.build) ? Build.fromJSON(object.build) : undefined,
      buildLargeFields: isSet(object.buildLargeFields) ? bytesFromBase64(object.buildLargeFields) : new Uint8Array(0),
      compression: isSet(object.compression) ? compressionFromJSON(object.compression) : 0,
      buildLargeFieldsDropped: isSet(object.buildLargeFieldsDropped)
        ? globalThis.Boolean(object.buildLargeFieldsDropped)
        : false,
    };
  },

  toJSON(message: BuildsV2PubSub): unknown {
    const obj: any = {};
    if (message.build !== undefined) {
      obj.build = Build.toJSON(message.build);
    }
    if (message.buildLargeFields.length !== 0) {
      obj.buildLargeFields = base64FromBytes(message.buildLargeFields);
    }
    if (message.compression !== 0) {
      obj.compression = compressionToJSON(message.compression);
    }
    if (message.buildLargeFieldsDropped !== false) {
      obj.buildLargeFieldsDropped = message.buildLargeFieldsDropped;
    }
    return obj;
  },

  create(base?: DeepPartial<BuildsV2PubSub>): BuildsV2PubSub {
    return BuildsV2PubSub.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<BuildsV2PubSub>): BuildsV2PubSub {
    const message = createBaseBuildsV2PubSub() as any;
    message.build = (object.build !== undefined && object.build !== null) ? Build.fromPartial(object.build) : undefined;
    message.buildLargeFields = object.buildLargeFields ?? new Uint8Array(0);
    message.compression = object.compression ?? 0;
    message.buildLargeFieldsDropped = object.buildLargeFieldsDropped ?? false;
    return message;
  },
};

function createBasePubSubCallBack(): PubSubCallBack {
  return { buildPubsub: undefined, userData: new Uint8Array(0) };
}

export const PubSubCallBack: MessageFns<PubSubCallBack> = {
  encode(message: PubSubCallBack, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.buildPubsub !== undefined) {
      BuildsV2PubSub.encode(message.buildPubsub, writer.uint32(10).fork()).join();
    }
    if (message.userData.length !== 0) {
      writer.uint32(18).bytes(message.userData);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): PubSubCallBack {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePubSubCallBack() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.buildPubsub = BuildsV2PubSub.decode(reader, reader.uint32());
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.userData = reader.bytes();
          continue;
        }
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skip(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): PubSubCallBack {
    return {
      buildPubsub: isSet(object.buildPubsub) ? BuildsV2PubSub.fromJSON(object.buildPubsub) : undefined,
      userData: isSet(object.userData) ? bytesFromBase64(object.userData) : new Uint8Array(0),
    };
  },

  toJSON(message: PubSubCallBack): unknown {
    const obj: any = {};
    if (message.buildPubsub !== undefined) {
      obj.buildPubsub = BuildsV2PubSub.toJSON(message.buildPubsub);
    }
    if (message.userData.length !== 0) {
      obj.userData = base64FromBytes(message.userData);
    }
    return obj;
  },

  create(base?: DeepPartial<PubSubCallBack>): PubSubCallBack {
    return PubSubCallBack.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<PubSubCallBack>): PubSubCallBack {
    const message = createBasePubSubCallBack() as any;
    message.buildPubsub = (object.buildPubsub !== undefined && object.buildPubsub !== null)
      ? BuildsV2PubSub.fromPartial(object.buildPubsub)
      : undefined;
    message.userData = object.userData ?? new Uint8Array(0);
    return message;
  },
};

function bytesFromBase64(b64: string): Uint8Array {
  if ((globalThis as any).Buffer) {
    return Uint8Array.from(globalThis.Buffer.from(b64, "base64"));
  } else {
    const bin = globalThis.atob(b64);
    const arr = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i);
    }
    return arr;
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if ((globalThis as any).Buffer) {
    return globalThis.Buffer.from(arr).toString("base64");
  } else {
    const bin: string[] = [];
    arr.forEach((byte) => {
      bin.push(globalThis.String.fromCharCode(byte));
    });
    return globalThis.btoa(bin.join(""));
  }
}

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}

export interface MessageFns<T> {
  encode(message: T, writer?: BinaryWriter): BinaryWriter;
  decode(input: BinaryReader | Uint8Array, length?: number): T;
  fromJSON(object: any): T;
  toJSON(message: T): unknown;
  create(base?: DeepPartial<T>): T;
  fromPartial(object: DeepPartial<T>): T;
}
