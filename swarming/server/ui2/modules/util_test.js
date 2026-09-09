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

import { sanitizeUrl } from "./util";

describe("util", function () {
  describe("sanitizeUrl", function () {
    it("accepts valid https URLs", function () {
      expect(
        sanitizeUrl(
          "https://chromium.googlesource.com/infra/luci/luci-go/+/deadbeef"
        )
      ).toBe("https://chromium.googlesource.com/infra/luci/luci-go/+/deadbeef");
      expect(sanitizeUrl("https://example.com/path?foo=bar#baz")).toBe(
        "https://example.com/path?foo=bar#baz"
      );
    });

    it("accepts valid http URLs", function () {
      expect(sanitizeUrl("http://localhost:8080/task")).toBe(
        "http://localhost:8080/task"
      );
      expect(sanitizeUrl("http://example.com")).toBe("http://example.com");
    });

    it("accepts safe same-origin relative paths", function () {
      expect(sanitizeUrl("/task?id=123")).toBe("/task?id=123");
      expect(sanitizeUrl("/raw/build/project/123/+/annotations")).toBe(
        "/raw/build/project/123/+/annotations"
      );
    });

    it("rejects javascript URLs", function () {
      expect(sanitizeUrl("javascript:alert(1)")).toBeUndefined();
      expect(sanitizeUrl("JAVASCRIPT:alert(1)")).toBeUndefined();
      expect(sanitizeUrl("   javascript:alert(1)   ")).toBeUndefined();
      expect(
        sanitizeUrl(
          "javascript:fetch('/auth/openid/state').then(r=>r.json())"
        )
      ).toBeUndefined();
    });

    it("rejects other unsafe schemes", function () {
      expect(
        sanitizeUrl("data:text/html,<script>alert(1)</script>")
      ).toBeUndefined();
      expect(sanitizeUrl("vbscript:msgbox(1)")).toBeUndefined();
      expect(sanitizeUrl("file:///etc/passwd")).toBeUndefined();
      expect(
        sanitizeUrl("blob:https://example.com/1234-5678")
      ).toBeUndefined();
    });

    it("rejects protocol-relative URLs", function () {
      expect(sanitizeUrl("//attacker.example/evil")).toBeUndefined();
    });

    it("rejects bare strings without explicit scheme or path", function () {
      expect(sanitizeUrl("deadbeef")).toBeUndefined();
      expect(sanitizeUrl("foo")).toBeUndefined();
      expect(sanitizeUrl("git@github.com:foo/bar.git")).toBeUndefined();
    });

    it("rejects non-string and falsy inputs", function () {
      expect(sanitizeUrl("")).toBeUndefined();
      expect(sanitizeUrl(null)).toBeUndefined();
      expect(sanitizeUrl(undefined)).toBeUndefined();
      expect(sanitizeUrl(12345)).toBeUndefined();
      expect(sanitizeUrl({})).toBeUndefined();
      expect(sanitizeUrl("not a valid url &&&")).toBeUndefined();
    });
  });
});
