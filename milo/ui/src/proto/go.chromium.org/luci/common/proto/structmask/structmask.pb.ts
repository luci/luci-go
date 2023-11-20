/* eslint-disable */
import _m0 from "protobufjs/minimal";

export const protobufPackage = "structmask";

/**
 * StructMask selects a subset of a google.protobuf.Struct.
 *
 * Usually used as a repeated field, to allow specifying a union of different
 * subsets.
 */
export interface StructMask {
  /**
   * A field path inside the struct to select.
   *
   * Each item can be:
   *   * `some_value` - a concrete dict key to follow (unless it is a number or
   *     includes `*`, use quotes in this case).
   *   * `"some_value"` - same, but quoted. Useful for selecting `*` or numbers
   *     literally. See https://pkg.go.dev/strconv#Unquote for syntax.
   *   * `<number>` (e.g. `0`) - a zero-based list index to follow.
   *     **Not implemented**.
   *   *  `*` - follow all dict keys and all list elements. Applies **only** to
   *     dicts and lists. Trying to recurse into a number or a string results
   *     in an empty match.
   *
   * When examining a value the following exceptional conditions result in
   * an empty match, which is represented by `null` for list elements or
   * omissions of the field for dicts:
   *   * Trying to follow a dict key while examining a list.
   *   * Trying to follow a key which is not present in the dict.
   *   * Trying to use `*` mask with values that aren't dicts or lists.
   *
   * When using `*`, the result is always a subset of the input. In particular
   * this is important when filtering lists: if a list of size N is selected by
   * the mask, then the filtered result will also always be a list of size N,
   * with elements filtered further according to the rest of the mask (perhaps
   * resulting in `null` elements on type mismatches, as explained above).
   */
  readonly path: readonly string[];
}

function createBaseStructMask(): StructMask {
  return { path: [] };
}

export const StructMask = {
  encode(message: StructMask, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.path) {
      writer.uint32(10).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StructMask {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStructMask() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.path.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): StructMask {
    return { path: globalThis.Array.isArray(object?.path) ? object.path.map((e: any) => globalThis.String(e)) : [] };
  },

  toJSON(message: StructMask): unknown {
    const obj: any = {};
    if (message.path?.length) {
      obj.path = message.path;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<StructMask>, I>>(base?: I): StructMask {
    return StructMask.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<StructMask>, I>>(object: I): StructMask {
    const message = createBaseStructMask() as any;
    message.path = object.path?.map((e) => e) || [];
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
