// Copyright 2019 The LUCI Authors.
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

describe("alias", function () {
  const { applyAlias } = require("modules/alias");

  describe("applyAlias", function () {
    it("maps device_type", function (done) {
      expect(applyAlias("blueline", "device_type")).toBe("Pixel 3 (blueline)");
      expect(applyAlias("iPhone9,1", "device_type")).toBe(
        "iPhone 7 (iPhone9,1)"
      );
      done();
    });

    it("maps gpu vendor", function (done) {
      expect(applyAlias("1002", "gpu")).toBe("AMD (1002)");
      expect(applyAlias("8086", "gpu")).toBe("Intel (8086)");
      done();
    });

    it("maps gpu device id", function (done) {
      expect(applyAlias("10de:1401", "gpu")).toBe(
        "NVIDIA GeForce GTX 960 (10de:1401)"
      );
      expect(applyAlias("8086:5912", "gpu")).toBe(
        "Intel Kaby Lake HD Graphics 630 (8086:5912)"
      );
      done();
    });

    it("maps gpu device id with driver", function (done) {
      expect(applyAlias("10de:1cb3-25.21.14.1678", "gpu")).toBe(
        "NVIDIA Quadro P400 (10de:1cb3-25.21.14.1678)"
      );
      expect(applyAlias("102b:0534-10.0.16299.15", "gpu")).toBe(
        "Matrox G200eR2 (102b:0534-10.0.16299.15)"
      );
      done();
    });

    it("maps os for Windows build numbers", function (done) {
      expect(applyAlias("Windows-10-15063", "os")).toBe(
        "Windows 10 version 1703 (Windows-10-15063)"
      );
      expect(applyAlias("Windows-10-17134.345", "os")).toBe(
        "Windows 10 version 1803 (Windows-10-17134.345)"
      );
      expect(applyAlias("Windows-11-26200", "os")).toBe(
        "Windows 11 version 25H2 (Windows-11-26200)"
      );
      expect(applyAlias("Windows-Server-14393", "os")).toBe(
        "Windows Server 2016 (Windows-Server-14393)"
      );
      expect(applyAlias("Windows-Server-17763.557", "os")).toBe(
        "Windows Server 2019 or version 1809 (Windows-Server-17763.557)"
      );
      done();
    });

    it("maps os for Mac versions", function (done) {
      expect(applyAlias("Mac-10.15.7", "os")).toBe(
        "macOS 10.15 Catalina (Mac-10.15.7)"
      );
      expect(applyAlias("Mac-14", "os")).toBe(
        "macOS 14 Sonoma (Mac-14)"
      );
      expect(applyAlias("Mac-15.1", "os")).toBe(
        "macOS 15 Sequoia (Mac-15.1)"
      );
      expect(applyAlias("Mac-26", "os")).toBe(
        "macOS 26 Tahoe (Mac-26)"
      );
      expect(applyAlias("Mac-27", "os")).toBe(
        "macOS 27 Golden Gate (Mac-27)"
      );
      done();
    });

    it("does not affect other os values", function (done) {
      const check = (v) => expect(applyAlias(v, "os")).toBe(v);
      check("Android");
      check("Debian-9.8");
      check("Mac-10.9.5");
      check("Windows");
      check("Windows-10");
      check("Windows-2016Server");
      check("Windows-8.1");
      check("Windows-8.1-SP0");
      done();
    });
  }); // end describe('applyAlias')
});
