/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

namespace LogDog {

  /** Log stream type RPC protobuf enumeration. */
  export enum StreamType {
    TEXT,
    BINARY,
    DATAGRAM,
  }

  /** Locally-typed LogStreamDescriptor RPC protobuf object class. */
  export class LogStreamDescriptor {
    readonly name: string;
    readonly timestamp: Date;
    readonly tags: {[key: string]: string};

    static make(desc: any): LogStreamDescriptor {
      desc.timestamp = new Date(desc.timestamp);
      return desc;
    }
  }

  /** Locally-typed LogStreamState RPC protobuf object class. */
  export class LogStreamState {
    created: Date;
    terminalIndex: number;
    archive: {};

    static make(state: any): LogStreamState {
      state.created = new Date(state.created);
      state.terminalIndex = int64(state.terminalIndex);
      return state;
    }
  }

  /** Locally-typed LogEntry RPC protobuf class with usability enhancements. */
  export class LogEntry {
    prefixIndex: number;
    streamIndex: number;
    timeOffset: number;

    /** The LogStreamDescriptor for this entry. */
    desc: LogStreamDescriptor|null;
    /** The timestamp of this specific LogEntry (base + offset). */
    timestamp: Date|null;

    /** Returns the text content of this log entry. */
    text?: {lines?: {value?: string; delimiter?: string;}[];};

    /** Builds a new LogEntry from a source protobuf and its descriptor. */
    static make(le: any, desc: LogStreamDescriptor): LogEntry {
      le.timeOffset = durationProtoToMillis(le.timeOffset);
      le.prefixIndex = int64(le.prefixIndex);
      le.streamIndex = int64(le.streamIndex);

      if (desc) {
        le.desc = desc;
        le.timestamp = addMillisecondsToDate(desc.timestamp, le.timeOffset);
      }
      return le;
    }
  }

  /**
   * A log stream project/path qualifier.
   *
   * The path component is either a full [prefix/+/name] path, or just the
   * [prefix] component.
   */
  export class StreamPath {
    /**
     * Construct a path from a project and path.
     *
     * @param project the project name
     * @param path the full stream path, or the prefix component of the stream
     *     path.
     */
    constructor(readonly project: string, readonly path: string) {}

    /**
     * @returns the full project-prefixed path: [project/path].
     */
    fullName(): string {
      return this.project + '/' + this.path;
    }

    /**
     * @returns the prefix component of the path.
     */
    get prefix(): string {
      let sepIdx = this.path.indexOf('/+');
      if (sepIdx > 0) {
        return this.path.substring(0, sepIdx);
      }
      return this.path;
    }

    /**
     * @returns the name component of the path, or an empty string if not
     *     specified.
     */
    get name(): string {
      const sep = '/+/';
      let sepIdx = this.path.indexOf(sep);
      if (sepIdx < 0) {
        return '';
      }
      return this.path.substring(sepIdx + sep.length);
    }

    /**
     * @returns true if "other" has the same prefix component as this
     *     StreamPath.
     */
    samePrefixAs(other: StreamPath): boolean {
      return (
          (this.project === other.project) && (this.prefix === other.prefix));
    }

    /**
     * Constructs a StreamPath from a project-prefixed path.
     *
     * For example, "foo/bar/+/baz" will constuct a StreamPath with project
     * "foo" and path "bar/+/baz".
     *
     * @param v the path-prefixed value.
     * @returns The parsed StreamPath.
     */
    static splitProject(v: string): StreamPath {
      let parts = StreamPath.split(v, 2);
      if (parts.length === 1) {
        return new StreamPath(v, '');
      }
      return new StreamPath(parts[0], parts[1]);
    }

    /**
     * Utility function to split a forward-slash-delimited value into an array
     * part strings.
     *
     * If count is 0, the array will have one element per path component. If
     * count is non-zero, the result will contain at most "count" elements. If
     * there are more than "count" elements, the last element will continue to
     * be slash-delimited.
     *
     * For example, 'foo/bar/baz' split with count 2 will return:
     * ['foo', 'bar/baz'].
     *
     * @param v the string to split.
     * @param count the maximum number of path elements to return, or 0 for all
     *     elements.
     * @returns An array of split path element strings.
     */
    static split(v: string, count: number): string[] {
      let parts = v.split('/');
      if (!count) {
        return parts;
      }
      let result = parts.splice(0, count - 1);
      if (parts.length) {
        result.push(parts.join('/'));
      }
      return result;
    };

    /**
     * Compares a StreamPath to another, returning its sort order.
     *
     * Sort happens first by project, then by path.
     *
     * @param a the left-hand path to compare.
     * @param b the right-hand path to compare.
     * @returns <0 if a < b, 0 if a == b, and >0 if a > b.
     */
    static cmp(a: StreamPath, b: StreamPath): number {
      if (a.project < b.project) {
        return -1;
      }
      if (a.project > b.project) {
        return 1;
      }
      return (a.path < b.path) ? -1 : ((a.path > b.path) ? 1 : 0);
    };
  }

  /**
   * Converts a string int64 into a Javascript number.
   *
   * Note that Javascript cannot hold a value larger than 2^53-1. If log streams
   * ever approach this length, we will need to rework this value as an integer-
   * string with helper functions.
   *
   * @param s the string to convert.
   * @returns the numeric value of the string.
   */
  function int64(s: string): number {
    if (!s) {
      return 0;
    }

    let value = parseInt(s, 10);
    if (isNaN(value)) {
      throw new Error('Value is not a number: ' + s);
    }
    return value;
  }

  /**
   * Adds a specified duration protobuf to the supplied Date.
   *
   * Duration protos are expressed as a string referencing a floating point
   * number of seconds followed by the letter "s":
   * - "1337s"
   * - "3.141592s"
   *
   * An undefined string will be interpreted as 0.
   *
   * @param value the time protobuf string to convert.
   * @returns the number of milliseconds indicated by the duration value.
   */
  function durationProtoToMillis(value: string): number {
    if (!value) {
      // Undefined / missing, use 0.
      return 0;
    }
    if (value.charAt(value.length - 1) !== 's') {
      throw new Error('Seconds string does not end in \'s\': ' + value);
    }
    return (parseFloat(value) * 1000.0);
  }

  /**
   * Returns a new Date object whose value is the initial date object with the
   * specified number of milliseconds added to it.
   *
   * @param d The base Date object.
   * @param ms The number of milliseconds to add.
   * @return a date that is "ms" milliseconds added to "d".
   */
  function addMillisecondsToDate(d: Date, ms: number) {
    d = new Date(d);
    d.setMilliseconds(d.getMilliseconds() + ms);
    return d;
  }
}
