// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// applyAlias will transform 'value' based on what 'key' is.
// This aliasing should be used for human-displayable only,
// not, for example, on the filters that are sent to the backend.
export function applyAlias(value, key) {
  if (!aliasMap[key] || value === "none" || !value) {
    return value;
  }
  let alias = aliasMap[key][value];
  if (key === "gpu") {
    // the longest gpu string looks like [pci id]-[driver version],
    // so we trim that off if needed.
    const trimmed = value.split("-")[0];
    alias = aliasMap[key][trimmed];
  } else if (key === "os") {
    // the Windows dimension ends with a build number, which may
    // include a minor version number corresponding to bugfix
    // releases. Only the major version of the build number
    // corresponds to a marketing name.
    if (value.startsWith("Windows")) {
      const trimmed = value.split(".")[0];
      alias = aliasMap[key][trimmed];
    }
  }
  if (!alias) {
    return value;
  }
  return `${alias} (${value})`;
}

const DEVICE_TYPE_ALIASES = {
  a13ve: "Galaxy A13",
  a23xq: "Galaxy A23",
  angler: "Nexus 6p",
  athene: "Moto G4",
  blueline: "Pixel 3",
  bullhead: "Nexus 5X",
  cheetah: "Pixel 7 Pro",
  crosshatch: "Pixel 3 XL",
  darcy: "NVIDIA Shield [2017]",
  dm1q: "Galaxy S23",
  dragon: "Pixel C",
  flame: "Pixel 4",
  flo: "Nexus 7 [2013]",
  flounder: "Nexus 9",
  foster: "NVIDIA Shield [2015]",
  fugu: "Nexus Player",
  gce_x86: "Android on GCE",
  goyawifi: "Galaxy Tab 3",
  grouper: "Nexus 7 [2012]",
  hammerhead: "Nexus 5",
  herolte: "Galaxy S7 [Global]",
  heroqlteatt: "Galaxy S7 [AT&T]",
  "iPad4,1": "iPad Air",
  "iPad5,1": "iPad mini 4",
  "iPad6,3": "iPad Pro [9.7-in]",
  "iPad7,5": "iPad [6th gen]",
  "iPad7,11": "iPad [7th gen]",
  "iPad8,9": "iPad Pro [11-in 2nd gen]",
  "iPad8,11": "iPad Pro [12.9-in 4th gen]",
  "iPad12,1": "iPad [9th gen]",
  "iPad13,4": "iPad Pro [11-in 3rd gen]",
  "iPad13,16": "iPad Air [5th gen]",
  "iPad14,3": "iPad Pro [11-in 4th gen]",
  "iPad14,5": "iPad Pro [12.9-in 6th gen]",
  "iPhone7,2": "iPhone 6",
  "iPhone9,1": "iPhone 7",
  "iPhone10,1": "iPhone 8",
  "iPhone12,1": "iPhone 11",
  "iPhone13,2": "iPhone 12",
  "iPhone14,2": "iPhone 13 Pro",
  "iPhone14,5": "iPhone 13",
  "iPhone16,1": "iPhone 15 Pro",
  j5xnlte: "Galaxy J5",
  m0: "Galaxy S3",
  mako: "Nexus 4",
  manta: "Nexus 10",
  marlin: "Pixel XL",
  mdarcy: "NVIDIA Shield [2019]",
  oriole: "Pixel 6",
  panther: "Pixel 7",
  raven: "Pixel 6 Pro",
  redfin: "Pixel 5",
  sailfish: "Pixel",
  sargo: "Pixel 3a",
  shamu: "Nexus 6",
  shiba: "Pixel 8",
  sprout: "Android One",
  starlte: "Galaxy S9",
  taimen: "Pixel 2 XL",
  tangorpro: "Pixel Tablet",
  "TECNO-KB8": "TECNO Spark 3 Pro",
  walleye: "Pixel 2",
  zerofltetmo: "Galaxy S6",
};

const GPU_ALIASES = {
  1002: "AMD",
  "1002:6613": "AMD Radeon R7 240",
  "1002:6646": "AMD Radeon R9 M280X",
  "1002:6779": "AMD Radeon HD 6450/7450/8450",
  "1002:67ef": "AMD Radeon Pro 560X",
  "1002:679e": "AMD Radeon HD 7800",
  "1002:6821": "AMD Radeon HD 8870M",
  "1002:683d": "AMD Radeon HD 7770/8760",
  "1002:7340": "AMD Radeon RX 5500 XT",
  "1002:7480": "AMD Radeon RX 7600",
  "1002:9830": "AMD Radeon HD 8400",
  "1002:9874": "AMD Carrizo",
  "1a03": "ASPEED",
  "1a03:2000": "ASPEED Graphics Family",
  "102b": "Matrox",
  "102b:0522": "Matrox MGA G200e",
  "102b:0532": "Matrox MGA G200eW",
  "102b:0534": "Matrox G200eR2",
  "10de": "NVIDIA",
  "10de:08a4": "NVIDIA GeForce 320M",
  "10de:08aa": "NVIDIA GeForce 320M",
  "10de:0a65": "NVIDIA GeForce 210",
  "10de:0fe9": "NVIDIA GeForce GT 750M Mac Edition",
  "10de:0ffa": "NVIDIA Quadro K600",
  "10de:104a": "NVIDIA GeForce GT 610",
  "10de:11c0": "NVIDIA GeForce GTX 660",
  "10de:1244": "NVIDIA GeForce GTX 550 Ti",
  "10de:1401": "NVIDIA GeForce GTX 960",
  "10de:1ba1": "NVIDIA GeForce GTX 1070",
  "10de:1cb3": "NVIDIA Quadro P400",
  "10de:2184": "NVIDIA GeForce GTX 1660",
  "10de:2783": "NVIDIA GeForce RTX 4070 Super",
  8086: "Intel",
  "8086:0046": "Intel Ironlake HD Graphics",
  "8086:0102": "Intel Sandy Bridge HD Graphics 2000",
  "8086:0116": "Intel Sandy Bridge HD Graphics 3000",
  "8086:0166": "Intel Ivy Bridge HD Graphics 4000",
  "8086:0412": "Intel Haswell HD Graphics 4600",
  "8086:041a": "Intel Haswell HD Graphics",
  "8086:0a16": "Intel Haswell HD Graphics 4400",
  "8086:0a26": "Intel Haswell HD Graphics 5000",
  "8086:0a2e": "Intel Haswell Iris Graphics 5100",
  "8086:0d26": "Intel Haswell Iris Pro Graphics 5200",
  "8086:0f31": "Intel Bay Trail HD Graphics",
  "8086:1616": "Intel Broadwell HD Graphics 5500",
  "8086:161e": "Intel Broadwell HD Graphics 5300",
  "8086:1626": "Intel Broadwell HD Graphics 6000",
  "8086:162b": "Intel Broadwell Iris Graphics 6100",
  "8086:1912": "Intel Skylake HD Graphics 530",
  "8086:191e": "Intel Skylake HD Graphics 515",
  "8086:1926": "Intel Skylake Iris 540/550",
  "8086:193b": "Intel Skylake Iris Pro 580",
  "8086:22b1": "Intel Braswell HD Graphics",
  "8086:3e92": "Intel Coffee Lake S UHD Graphics 630",
  "8086:3e9b": "Intel Coffee Lake H UHD Graphics 630",
  "8086:3ea5": "Intel Coffee Lake Iris Plus Graphics 655",
  "8086:4680": "Intel Alder Lake S UHD Graphics 770",
  "8086:5912": "Intel Kaby Lake HD Graphics 630",
  "8086:591e": "Intel Kaby Lake HD Graphics 615",
  "8086:5926": "Intel Kaby Lake Iris Plus Graphics 640",
  "8086:9bc5": "Intel Comet Lake S UHD Graphics 630",
  qcom: "Qualcomm",
  "qcom:043a": "Qualcomm Adreno 690",
};

const DEVICE_ALIASES = {
  "iPad4,1": "iPad Air",
  "iPad5,1": "iPad mini 4",
  "iPad6,3": "iPad Pro [9.7 in]",
  "iPhone7,2": "iPhone 6",
  "iPhone9,1": "iPhone 7",
};

/** For Win10, the correspondence between build numbers and versions
 *  is published at
 *  https://docs.microsoft.com/en-us/windows/release-information/ for
 *  clients and at
 *  https://docs.microsoft.com/en-us/windows-server/get-started/windows-server-release-info
 *  for servers. This mapping will need to be updated for each new
 *  Win10 release.
 */
const OS_ALIASES = {
  "Ubuntu-14.04": "Ubuntu 14.04 Trusty Tahr",
  "Ubuntu-16.04": "Ubuntu 16.04 Xenial Xerus",
  "Ubuntu-18.04": "Ubuntu 18.04 Bionic Beaver",
  "Ubuntu-20.04": "Ubuntu 20.04 Focal Fossa",
  "Ubuntu-22.04": "Ubuntu 22.04 Jammy Jellyfish",
  "Ubuntu-24.04": "Ubuntu 24.04 Noble Numbat",
  "Windows-10-10240": "Windows 10 version 1507",
  "Windows-10-10586": "Windows 10 version 1511",
  "Windows-10-14393": "Windows 10 version 1607",
  "Windows-10-15063": "Windows 10 version 1703",
  "Windows-10-16299": "Windows 10 version 1709",
  "Windows-10-17134": "Windows 10 version 1803",
  "Windows-10-17763": "Windows 10 version 1809",
  "Windows-10-18362": "Windows 10 version 1903",
  "Windows-10-18363": "Windows 10 version 1909",
  "Windows-10-19042": "Windows 10 version 20H2",
  "Windows-10-19043": "Windows 10 version 21H1",
  "Windows-10-19044": "Windows 10 version 21H2",
  "Windows-10-19045": "Windows 10 version 22H2",
  "Windows-11-22000": "Windows 11 version 21H2",
  "Windows-11-22621": "Windows 11 version 22H2",
  "Windows-11-22631": "Windows 11 version 23H2",
  "Windows-11-26100": "Windows 11 version 24H2",
  "Windows-Server-14393": "Windows Server 2016",
  "Windows-Server-17134": "Windows Server version 1803",
  "Windows-Server-17763": "Windows Server 2019 or version 1809",
  "Windows-Server-18362": "Windows Server version 1903",
  "Windows-Server-18363": "Windows Server version 1909",
};

const aliasMap = {
  device: DEVICE_ALIASES,
  device_type: DEVICE_TYPE_ALIASES,
  gpu: GPU_ALIASES,
  os: OS_ALIASES,
};

/** maybeApplyAlias will take a filter (e.g. foo:bar) and apply
 *  the alias to it inline, returning it to be displayed on the UI.
 *  This means we can display it in a human-friendly way, without
 *  the complexity of handling the alias when storing URL params
 *  or making API requests.
 */
export function maybeApplyAlias(filter) {
  const idx = filter.indexOf(":");
  if (idx < 0) {
    return filter;
  }
  const key = filter.substring(0, idx);
  const value = filter.substring(idx + 1);
  // remove -tag for tasks if it exists
  const trimmed = key.split("-tag")[0];
  return `${key}:${applyAlias(value, trimmed)}`;
}
