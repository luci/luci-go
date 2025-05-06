// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
      expect(applyAlias("Windows-Server-14393", "os")).toBe(
        "Windows Server 2016 (Windows-Server-14393)"
      );
      expect(applyAlias("Windows-Server-17763.557", "os")).toBe(
        "Windows Server 2019 or version 1809 (Windows-Server-17763.557)"
      );
      done();
    });

    it("does not affect other os values", function (done) {
      const check = (v) => expect(applyAlias(v, "os")).toBe(v);
      check("Android");
      check("Debian-9.8");
      check("Mac-10.11.6");
      check("Windows");
      check("Windows-10");
      check("Windows-2016Server");
      check("Windows-8.1");
      check("Windows-8.1-SP0");
      done();
    });
  }); // end describe('applyAlias')
});
