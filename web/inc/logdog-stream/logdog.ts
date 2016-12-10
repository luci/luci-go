/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

export module LogDog {
  /** Locally-typed LogStreamDescriptor class. */
  export interface LogStreamDescriptor {
    readonly timestamp: Date;
  }

  export function makeLogStreamDescriptor(desc: any): LogStreamDescriptor {
    desc.timestamp = new Date(desc.timestamp);
    return desc;
  }

  export interface LogStreamState {
    created: Date;
    terminalIndex: number;
    archive: {

    };
  }

  export function makeLogStreamState(state: any): LogStreamState {
    state.created = new Date(state.created);
    state.terminalIndex = int64(state.terminalIndex);
    return state;
  }

  /** Locally-typed LogEntry class. */
  export type LogEntry = {
    prefixIndex: number;
    streamIndex: number;
    timeOffset: number;

    desc: LogStreamDescriptor | null;
    timestamp: Date | null;

    text?: {
      lines?: {
        value?: string;
        delimiter?: string;
      }[];
    };
  }

  export function makeLogEntry(le: any, desc: LogStreamDescriptor): LogEntry {
    le.timeOffset = durationProtoToMillis(le.timeOffset);
    le.prefixIndex = int64(le.prefixIndex);
    le.streamIndex = int64(le.streamIndex);

    if (desc) {
      le.desc = desc;
      le.timestamp = addMillisecondsToDate(desc.timestamp, le.timeOffset);
    }
    return le;
  }

  /** A log stream project/path qualifier. */
  export class Stream {
    constructor(readonly project: string, readonly path: string) {}

    fullName(): string {
      return this.project + "/" + this.path;
    }

    get prefix(): string {
      let sepIdx = this.path.indexOf("/+");
      if (sepIdx > 0) {
        return this.path.substring(0, sepIdx);
      }
      return this.path;
    }

    get name(): string {
      const sep = "/+/";
      let sepIdx = this.path.indexOf(sep);
      if (sepIdx < 0) {
        return "";
      }
      return this.path.substring(sepIdx + sep.length);
    }

    samePrefixAs(other: Stream): boolean {
      return (
          (this.project === other.project) &&
          (this.prefix === other.prefix));
    }

    static splitProject(v: string): Stream {
      let parts = Stream.split(v, 2);
      if (parts.length === 1) {
        return new Stream(v, "");
      }
      return new Stream(parts[0], parts[1]);
    }

    static split = function(v: string, count: number): string[] {
      let parts = v.split("/");
      if (!count) {
        return parts;
      }
      let result = parts.splice(0, count-1);
      if (parts.length) {
        result.push(parts.join("/"));
      }
      return result;
    };

    static cmp(a: Stream, b: Stream): number {
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
   */
  function int64(s: string): number {
    if (!s) {
      return 0;
    }

    let value = parseInt(s, 10);
    if (isNaN(value)) {
      throw new Error("Value is not a number: " + s);
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
   */
  function durationProtoToMillis(value: string): number {
    if ((!value) || value.charAt(value.length - 1) !== "s") {
      throw new Error("Seconds string does not end in 's': " + value);
    }
    return (parseFloat(value) * 1000.0);
  }

  /**
   * Returns a new Date object whose value is the initial date object with the
   * specified number of milliseconds added to it.
   *
   * @param d {Date} The base Date object.
   * @param ms {Number} The number of milliseconds to add.
   */
  function addMillisecondsToDate(d: Date, ms: number) {
    d = new Date(d);
    d.setMilliseconds(d.getMilliseconds() + ms);
    return d;
  }
}
