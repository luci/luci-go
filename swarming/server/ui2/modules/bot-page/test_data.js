// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

export function botData(url, opts) {
  let botId = url.match("/bot/(.+)/get");
  botId = botId[1] || "running";
  return botDataMap[botId];
}

export const botDataMap = {
  running: {
    authenticatedAs: "bot:running.chromium.org",
    dimensions: [
      {
        value: ["vpython"],
        key: "caches",
      },
      {
        value: ["8"],
        key: "cores",
      },
      {
        value: ["x86", "x86-64", "x86-64-E3-1230_v5"],
        key: "cpu",
      },
      {
        value: ["0"],
        key: "gce",
      },
      {
        value: ["10de", "10de:1cb3", "10de:1cb3-25.21.14.1678"],
        key: "gpu",
      },
      {
        value: ["build16-a9"],
        key: "id",
      },
      {
        value: ["high"],
        key: "integrity",
      },
      {
        value: ["en_US.cp1252"],
        key: "locale",
      },
      {
        value: ["n1-standard-8"],
        key: "machine_type",
      },
      {
        value: ["Windows", "Windows-10", "Windows-10-16299.309"],
        key: "os",
      },
      {
        value: ["Skia"],
        key: "pool",
      },
      {
        value: ["2.7.13"],
        key: "python",
      },
      {
        value: ["1"],
        key: "rack",
      },
      {
        value: ["4085-c81638b"],
        key: "server_version",
      },
      {
        value: [
          "us",
          "us-mtv",
          "us-mtv-chops",
          "us-mtv-chops-a",
          "us-mtv-chops-a-9",
        ],
        key: "zone",
      },
    ],
    taskId: "42fb00e06d95be11",
    externalIp: "70.32.137.220",
    isDead: false,
    quarantined: false,
    deleted: false,
    state:
      '{"audio":["NVIDIA High Definition Audio"],"bot_group_cfg_version":"hash:d50e0a198b5ee4","cost_usd_hour":0.7575191297743056,"cpu_name":"Intel(R) Xeon(R) CPU E3-1230 v5 @ 3.40GHz","cwd":"C:\\\\b\\\\s","cygwin":[false],"disks":{"c:\\\\":{"free_mb":690166.5,"size_mb":763095.0}},"env":{"PATH":"C:\\\\Windows\\\\system32;C:\\\\Windows;C:\\\\Windows\\\\System32\\\\Wbem;C:\\\\Windows\\\\System32\\\\WindowsPowerShell\\\\v1.0\\\\;c:\\\\Tools;C:\\\\CMake\\\\bin;C:\\\\Program Files\\\\Puppet Labs\\\\Puppet\\\\bin;C:\\\\Users\\\\chrome-bot\\\\AppData\\\\Local\\\\Microsoft\\\\WindowsApps"},"files":{"c:\\\\Users\\\\chrome-bot\\\\ntuser.dat":1310720},"gpu":["Nvidia Quadro P400 25.21.14.1678"],"hostname":"build16-a9.labs.chromium.org","ip":"192.168.216.26","named_caches":{"vpython":[["qp",50935560],1549982906.0]},"nb_files_in_temp":2,"pid":7940,"python":{"executable":"c:\\\\infra-system\\\\bin\\\\python.exe","packages":null,"version":"2.7.13 (v2.7.13:a06454b1afa1, Dec 17 2016, 20:53:40) [MSC v.1500 64 bit (AMD64)]"},"ram":32726,"running_time":21321,"sleep_streak":8,"ssd":[],"started_ts":1549961665,"top_windows":[],"uptime":21340,"user":"chrome-bot"}',
    version: "8ea94136c96de7396fda8587d8e40cbc2d0c20ec01ce6b45c68d42a526d02316",
    firstSeenTs: "2017-08-02T23:12:16.365500",
    taskName: "Test-Win10-Clang-Golo-GPU-QuadroP400-x86_64-Debug-All-ANGLE",
    lastSeenTs: "2019-02-12T14:54:12.335408",
    botId: "running",
  },
  quarantined: {
    authenticatedAs:
      "bot-with-really-long-service-account-name:running.chromium.org",
    dimensions: [
      {
        value: ["1"],
        key: "android_devices",
      },
      {
        value: ["vpython"],
        key: "caches",
      },
      {
        value: ["ondemand"],
        key: "cpu_governor",
      },
      {
        value: ["12.5.21"],
        key: "device_gms_core_version",
      },
      {
        value: ["O", "OPR2.170623.027"],
        key: "device_os",
      },
      {
        value: ["google"],
        key: "device_os_flavor",
      },
      {
        value: ["9.2.32-xhdpi"],
        key: "device_playstore_version",
      },
      {
        value: ["brcm"],
        key: "device_tree_compatible",
      },
      {
        value: ["fugu"],
        key: "device_type",
      },
      {
        value: ["0"],
        key: "gce",
      },
      {
        value: ["quarantined"],
        key: "id",
      },
      {
        value: ["0"],
        key: "inside_docker",
      },
      {
        value: ["Android"],
        key: "os",
      },
      {
        value: ["Skia"],
        key: "pool",
      },
      {
        value: ["2.7.9"],
        key: "python",
      },
      {
        value: ["4098-34330fc"],
        key: "server_version",
      },
      {
        value: ["us", "us-skolo", "us-skolo-1"],
        key: "zone",
      },
    ],
    taskId: "",
    externalIp: "100.115.95.143",
    isDead: false,
    quarantined: true,
    deleted: false,
    state:
      '{"audio":null,"bot_group_cfg_version":"hash:0d12ff88393b4d","cost_usd_hour":0.15235460069444445,"cpu_name":"BCM2709","cwd":"/b/s","devices":{"3BE9F057":{"battery":{"current":null,"health":2,"level":100,"power":["AC"],"status":2,"temperature":424,"voltage":0},"build":{"board.platform":"<missing>","build.fingerprint":"google/fugu/fugu:8.0.0/OPR2.170623.027/4397545:userdebug/dev-keys","build.id":"OPR2.170623.027","build.product":"fugu","build.version.sdk":"26","product.board":"fugu","product.cpu.abi":"x86","product.device":"fugu"},"cpu":{"cur":"1833000","governor":"interactive"},"disk":{},"imei":null,"ip":[],"max_uid":null,"mem":{},"other_packages":["com.intel.thermal","android.autoinstalls.config.google.fugu"],"port_path":"1/4","processes":2,"state":"still booting (sys.boot_completed)","temp":{},"uptime":129027.39}},"disks":{"/b":{"free_mb":4314.0,"size_mb":26746.5},"/boot":{"free_mb":40.4,"size_mb":59.9},"/home/chrome-bot":{"free_mb":986.2,"size_mb":988.9},"/tmp":{"free_mb":974.6,"size_mb":975.9},"/var":{"free_mb":223.6,"size_mb":975.9}},"env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/games:/usr/games"},"gpu":null,"host_dimensions":{"caches":["vpython"],"cores":["4"],"cpu":["arm","arm-32","armv7l","armv7l-32","armv7l-32-BCM2709"],"cpu_governor":["ondemand"],"device_tree_compatible":["brcm"],"gce":["0"],"gpu":["none"],"id":["quarantined"],"inside_docker":["0"],"kvm":["0"],"machine_type":["n1-highcpu-4"],"os":["Linux","Raspbian","Raspbian-8.0"],"python":["2.7.9"],"ssd":["1"]},"hostname":"quarantined","ip":"192.168.1.152","named_caches":{"vpython":[["sQ",92420605],1550019574.0]},"nb_files_in_temp":6,"pid":499,"python":{"executable":"/usr/bin/python","packages":["M2Crypto==0.21.1","RPi.GPIO==0.6.3","argparse==1.2.1","chardet==2.3.0","colorama==0.3.2","html5lib==0.999","libusb1==1.5.0","ndg-httpsclient==0.3.2","pyOpenSSL==0.13.1","pyasn1==0.1.7","requests==2.4.3","rsa==3.4.2","six==1.8.0","urllib3==1.9.1","wheel==0.24.0","wsgiref==0.1.2"],"version":"2.7.9 (default, Sep 17 2016, 20:26:04) \\n[GCC 4.9.2]"},"quarantined":"No available devices.","ram":926,"running_time":23283,"sleep_streak":63,"ssd":["mmcblk0"],"started_ts":1550125333,"temp":{"thermal_zone0":47.774},"uptime":23954,"user":"chrome-bot"}',
    version: "f775dd9893167e6fee31b96ef20f7218f07fa437ea9d6fc44496208784108545",
    firstSeenTs: "2016-09-09T21:05:34.439930",
    lastSeenTs: "2019-02-12T12:50:20.961462",
    botId: "quarantined",
  },
  dead: {
    authenticatedAs: "bot:running.chromium.org",
    dimensions: [
      {
        value: ["swarming_module_cache_vpython"],
        key: "caches",
      },
      {
        value: ["8"],
        key: "cores",
      },
      {
        value: ["x86", "x86-64", "x86-64-Broadwell_GCE", "x86-64-avx2"],
        key: "cpu",
      },
      {
        value: ["1"],
        key: "gce",
      },
      {
        value: ["none"],
        key: "gpu",
      },
      {
        value: ["dead"],
        key: "id",
      },
      {
        value: ["chrome-trusty-18091700-38cc06ee3ee"],
        key: "image",
      },
      {
        value: ["0"],
        key: "inside_docker",
      },
      {
        value: ["1"],
        key: "kvm",
      },
      {
        value: ["n1-standard-8"],
        key: "machine_type",
      },
      {
        value: ["Linux", "Ubuntu", "Ubuntu-14.04"],
        key: "os",
      },
      {
        value: ["Chrome"],
        key: "pool",
      },
      {
        value: ["2.7.6"],
        key: "python",
      },
      {
        value: ["3986-3c043d8"],
        key: "server_version",
      },
      {
        value: ["us", "us-east", "us-east1", "us-east1-b"],
        key: "zone",
      },
    ],
    taskId: "",
    externalIp: "35.229.11.33",
    isDead: true,
    deleted: false,
    quarantined: false,
    state:
      '{"audio":[],"bot_group_cfg_version":"hash:5bbd7d8f05c65e","cost_usd_hour":0.41316150716145833,"cpu_name":"Intel(R) Xeon(R) CPU Broadwell GCE","cwd":"/b/s","disks":{"/":{"free_mb":246117.8,"size_mb":302347.0}},"env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["Gubbi","Navilu","dejavu","fonts-japanese-gothic.ttf","fonts-japanese-mincho.ttf","kochi","liberation","msttcorefonts","pagul","tlwg","ttf-bengali-fonts","ttf-dejavu","ttf-devanagari-fonts","ttf-gujarati-fonts","ttf-indic-fonts-core","ttf-kannada-fonts","ttf-malayalam-fonts","ttf-oriya-fonts","ttf-punjabi-fonts","ttf-tamil-fonts","ttf-telugu-fonts"]},"gpu":[],"hostname":"dead.us-east1-b.c.chromecompute.google.com.internal","ip":"10.0.8.219","named_caches":{"swarming_module_cache_vpython":[["kL",887763447],1547511540.0]},"nb_files_in_temp":8,"pid":1117,"python":{"executable":"/usr/bin/python","packages":["Cheetah==2.4.4","CherryPy==3.2.2","Landscape-Client==14.12","PAM==0.4.2","PyYAML==3.10","Routes==2.0","Twisted-Core==13.2.0","Twisted-Names==13.2.0","Twisted-Web==13.2.0","WebOb==1.3.1","apt-xapian-index==0.45","argparse==1.2.1","boto==2.20.1","chardet==2.0.1","cloud-init==0.7.5","colorama==0.2.5","configobj==4.7.2","coverage==3.7.1","crcmod==1.7","google-compute-engine==2.2.4","html5lib==0.999","iotop==0.6","jsonpatch==1.3","jsonpointer==1.0","numpy==1.8.2","oauth==1.0.1","pexpect==3.1","prettytable==0.7.2","psutil==1.2.1","pyOpenSSL==0.13","pycrypto==2.6.1","pycurl==7.19.3","pyserial==2.6","python-apt==0.9.3.5ubuntu2","python-debian==0.1.21-nmu2ubuntu2","pyxdg==0.25","repoze.lru==0.6","requests==2.2.1","six==1.5.2","ssh-import-id==3.21","urllib3==1.7.1","wheel==0.24.0","wsgiref==0.1.2","zope.interface==4.0.5"],"version":"2.7.6 (default, Nov 13 2018, 12:45:42) \\n[GCC 4.8.4]"},"ram":30159,"running_time":7309,"sleep_streak":22,"ssd":[],"started_ts":1547505598,"uptime":7321,"user":"chrome-bot"}',
    version: "9644ba2fcbeafe7628828602251e5405db3d79b9cd230523bdf7927e204d664e",
    firstSeenTs: "2019-01-14T00:40:11.400947",
    lastSeenTs: "2019-01-15T00:42:19.613017",
    botId: "dead",
  },
};

// These lists are really long, and likely will not have the data modified,
// so it doesn't make much to pretty-print them.
export const tasksMap = {
  SkiaGPU: [
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "fRHRPnwD-QYB0Bz5h15rZPC8Lw0I4Kwq-LL3TGDj0lQC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "wtLX1NImdsT8plyqaOCZZqKsrRWFjPrjGqDoCTkPEdUC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "0URK7JefXkIaBhfPV_cNHNLD1Pg3pjKiESFgfvizHg4C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "-9dcmmPEPvG5lS2DmocBNzwAS4ZhdGuKtH3gNA_WcbUC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "pBBZCIJTY2AA51MGjLR06EitVyGLmKsLodv2Le2n51oC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "03FEaLHT7gyS0pLyUKkeG-3g1OoG1r6Jhy5XJBlRtdMC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "hSkTe2cH1nI82U-qCT_9dJQ9s3-VSzelZ1gQHtdujucC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "kBmfswcNsndZHYCnIK7FuRxcsn0u8RWmQE_GkbOmR-EC",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "cZftrW_HII3ma82AxdFVKu43dKPGxMboC5x6tWl3jzgC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "Ia4Fa36LF61grdNFj_GEgK0zSP6M4izt78-NB935N-IC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "VbljnL3ZZm_VvRM-MsxfMpDPQ2m2kL--haq7V0xMkfwC",
          },
        ],
      },
      name: "post task for flash build243-m4--device1 to N2G48C",
      createdTs: "2023-04-20T23:38:27.381574Z",
      startedTs: "2023-04-20T23:39:21.231269Z",
      modifiedTs: "2023-04-20T23:39:34.442609Z",
      runId: "61b9da1ebd045411",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0009711399],
      completedTs: "2023-04-20T23:39:34.442609Z",
      botIdleSinceTs: "2023-04-20T23:30:49.934937Z",
      serverVersions: ["7150-82ce7e0"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7150-82ce7e0"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "61b9da1ebd045411",
      duration: 0.004925251,
      performanceStats: {
        botOverhead: 5.65246,
        namedCachesInstall: {
          duration: 0.00054621696,
        },
        namedCachesUninstall: {
          duration: 0.07203245,
        },
        cleanup: {
          duration: 0.3791771,
        },
        cacheTrim: {
          duration: 0.00040721893,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 4.085696,
        },
      },
      botVersion:
        "a99c78a5c7a6114cb4a4aeb818b103c587d3d5cf69aee10cc32bb76e42a85a61",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "fRHRPnwD-QYB0Bz5h15rZPC8Lw0I4Kwq-LL3TGDj0lQC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "wtLX1NImdsT8plyqaOCZZqKsrRWFjPrjGqDoCTkPEdUC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "0URK7JefXkIaBhfPV_cNHNLD1Pg3pjKiESFgfvizHg4C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "-9dcmmPEPvG5lS2DmocBNzwAS4ZhdGuKtH3gNA_WcbUC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "pBBZCIJTY2AA51MGjLR06EitVyGLmKsLodv2Le2n51oC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "03FEaLHT7gyS0pLyUKkeG-3g1OoG1r6Jhy5XJBlRtdMC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "hSkTe2cH1nI82U-qCT_9dJQ9s3-VSzelZ1gQHtdujucC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "kBmfswcNsndZHYCnIK7FuRxcsn0u8RWmQE_GkbOmR-EC",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "cZftrW_HII3ma82AxdFVKu43dKPGxMboC5x6tWl3jzgC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "Ia4Fa36LF61grdNFj_GEgK0zSP6M4izt78-NB935N-IC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "VbljnL3ZZm_VvRM-MsxfMpDPQ2m2kL--haq7V0xMkfwC",
          },
        ],
      },
      name: "flash build243-m4--device1 to N2G48C",
      createdTs: "2023-04-20T23:20:55.434801Z",
      startedTs: "2023-04-20T23:21:37.581191Z",
      modifiedTs: "2023-04-20T23:29:05.678339Z",
      runId: "61b9ca11914d5d11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.034229606],
      completedTs: "2023-04-20T23:29:05.678339Z",
      botIdleSinceTs: "2023-04-20T21:12:29.174257Z",
      serverVersions: ["7150-82ce7e0"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7150-82ce7e0"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "61b9ca11914d5d11",
      duration: 433.82755,
      performanceStats: {
        botOverhead: 9.260204,
        namedCachesInstall: {
          duration: 0.00027298927,
        },
        namedCachesUninstall: {
          duration: 0.07950902,
        },
        cleanup: {
          duration: 1.2331481,
        },
        cacheTrim: {
          duration: 0.00044369698,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 6.8957872,
        },
      },
      botVersion:
        "a99c78a5c7a6114cb4a4aeb818b103c587d3d5cf69aee10cc32bb76e42a85a61",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "Eilcyxhv5aw4AmUKwgp3m9JpPGq3xlVkb4_mWf0zpcAC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "mZx_7LvYy-gsu4GKwUyBD1z_BjZ0AL8Vq09Zic3WuFsC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "qOzRW0R__BIvU9sNpSp4PyFRsV3ZLJEnoS8bkURRP-oC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "w1lf8GGQpo2pD_w8VxFX6JXRNxVcFNhg_gmYwvOgOdkC",
          },
        ],
      },
      name: "Flash build243-m4--device1 (bullhead) to N2G48C",
      createdTs: "2023-03-20T21:54:31.772009Z",
      startedTs: "2023-03-20T21:55:47.491289Z",
      modifiedTs: "2023-03-20T21:56:00.302732Z",
      runId: "6119d5d4ec186511",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.000953692],
      completedTs: "2023-03-20T21:56:00.302732Z",
      botIdleSinceTs: "2023-03-20T19:08:03.254187Z",
      failure: true,
      state: "COMPLETED",
      serverVersions: ["7078-115fec4"],
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7078-115fec4"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "6119d5d4ec186511",
      duration: 0.0051431656,
      performanceStats: {
        botOverhead: 5.3218584,
        namedCachesInstall: {
          duration: 0.0005335808,
        },
        namedCachesUninstall: {
          duration: 0.056872845,
        },
        cleanup: {
          duration: 0.38282394,
        },
        cacheTrim: {
          duration: 0.00041675568,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.948864,
        },
      },
      exitCode: 127,
      botVersion:
        "ddf0f9f11cdd69eb0e93336b2ebef48350a61a07a99b29582f7f6cde49c5274e",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "Eilcyxhv5aw4AmUKwgp3m9JpPGq3xlVkb4_mWf0zpcAC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "mZx_7LvYy-gsu4GKwUyBD1z_BjZ0AL8Vq09Zic3WuFsC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
        ],
      },
      name: "Flash build243-m4--device1 (bullhead) to N2G48C",
      createdTs: "2023-03-20T18:53:32.643929Z",
      startedTs: "2023-03-20T18:53:33.915474Z",
      modifiedTs: "2023-03-20T18:53:46.871580Z",
      runId: "611930226c10c211",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.00096181495],
      completedTs: "2023-03-20T18:53:46.871580Z",
      botIdleSinceTs: "2023-03-20T18:11:55.050009Z",
      failure: true,
      state: "COMPLETED",
      serverVersions: ["7079-22bface-justin-fix-double-bot-task"],
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7079-22bface-justin-fix-double-bot-task"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "611930226c10c211",
      duration: 0.004949808,
      performanceStats: {
        botOverhead: 5.6221504,
        namedCachesInstall: {
          duration: 0.0005133152,
        },
        namedCachesUninstall: {
          duration: 0.056437492,
        },
        cleanup: {
          duration: 0.22286868,
        },
        cacheTrim: {
          duration: 0.00046682358,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 4.3037453,
        },
      },
      exitCode: 127,
      botVersion:
        "c42e4f58b9d0219a1be043d318bbe7f879a6f664bc932e7801e18d643fb558b9",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "Eilcyxhv5aw4AmUKwgp3m9JpPGq3xlVkb4_mWf0zpcAC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "mZx_7LvYy-gsu4GKwUyBD1z_BjZ0AL8Vq09Zic3WuFsC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
        ],
      },
      name: "Flash build243-m4--device1 (bullhead) to N2G48C",
      createdTs: "2023-03-20T18:10:01.089308Z",
      startedTs: "2023-03-20T18:10:14.375909Z",
      modifiedTs: "2023-03-20T18:10:28.861009Z",
      runId: "611908493c8f7811",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0010734567],
      completedTs: "2023-03-20T18:10:28.861009Z",
      botIdleSinceTs: "2023-03-20T16:11:52.416144Z",
      failure: true,
      state: "COMPLETED",
      serverVersions: ["7079-22bface-justin-fix-double-bot-task"],
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7079-22bface-justin-fix-double-bot-task"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "611908493c8f7811",
      duration: 0.0063221455,
      performanceStats: {
        botOverhead: 6.889406,
        namedCachesInstall: {
          duration: 0.014932871,
        },
        namedCachesUninstall: {
          duration: 0.29463148,
        },
        cleanup: {
          duration: 0.2137618,
        },
        cacheTrim: {
          duration: 0.0004248619,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 5.345985,
        },
      },
      exitCode: 127,
      botVersion:
        "c42e4f58b9d0219a1be043d318bbe7f879a6f664bc932e7801e18d643fb558b9",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "Eilcyxhv5aw4AmUKwgp3m9JpPGq3xlVkb4_mWf0zpcAC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "mZx_7LvYy-gsu4GKwUyBD1z_BjZ0AL8Vq09Zic3WuFsC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
        ],
      },
      name: "Flash build243-m4--device1 (bullhead) to N2G48C",
      createdTs: "2023-03-18T01:11:05.210585Z",
      startedTs: "2023-03-18T01:11:27.030710Z",
      modifiedTs: "2023-03-18T01:11:38.408625Z",
      runId: "610b16b503ef7111",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.00084111304],
      completedTs: "2023-03-18T01:11:38.408625Z",
      botIdleSinceTs: "2023-03-17T23:37:00.428393Z",
      serverVersions: ["7078-115fec4"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7078-115fec4"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "610b16b503ef7111",
      duration: 0.0047035217,
      performanceStats: {
        botOverhead: 4.566142,
        namedCachesInstall: {
          duration: 0.00053334236,
        },
        namedCachesUninstall: {
          duration: 0.055940866,
        },
        cleanup: {
          duration: 0.2115295,
        },
        cacheTrim: {
          duration: 0.00040888786,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.2303605,
        },
      },
      botVersion:
        "ddf0f9f11cdd69eb0e93336b2ebef48350a61a07a99b29582f7f6cde49c5274e",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "Eilcyxhv5aw4AmUKwgp3m9JpPGq3xlVkb4_mWf0zpcAC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "mZx_7LvYy-gsu4GKwUyBD1z_BjZ0AL8Vq09Zic3WuFsC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
        ],
      },
      name: "Flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-17T23:36:02.052170Z",
      startedTs: "2023-03-17T23:36:12.684292Z",
      modifiedTs: "2023-03-17T23:36:27.800445Z",
      runId: "610abfaf0c143b11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.001125602],
      completedTs: "2023-03-17T23:36:27.800445Z",
      botIdleSinceTs: "2023-03-17T22:57:01.540430Z",
      serverVersions: ["7078-115fec4"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7078-115fec4"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "610abfaf0c143b11",
      duration: 0.006629944,
      performanceStats: {
        botOverhead: 7.7490716,
        namedCachesInstall: {
          duration: 0.012924194,
        },
        namedCachesUninstall: {
          duration: 0.5383549,
        },
        cleanup: {
          duration: 0.20988607,
        },
        cacheTrim: {
          duration: 0.00064611435,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 5.992985,
        },
      },
      botVersion:
        "ddf0f9f11cdd69eb0e93336b2ebef48350a61a07a99b29582f7f6cde49c5274e",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "Eilcyxhv5aw4AmUKwgp3m9JpPGq3xlVkb4_mWf0zpcAC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "mZx_7LvYy-gsu4GKwUyBD1z_BjZ0AL8Vq09Zic3WuFsC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "w1lf8GGQpo2pD_w8VxFX6JXRNxVcFNhg_gmYwvOgOdkC",
          },
        ],
      },
      name: "Flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-17T01:34:21.494460Z",
      startedTs: "2023-03-17T01:34:43.205914Z",
      modifiedTs: "2023-03-17T01:39:16.897252Z",
      runId: "610605a76664cd11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.020886749],
      completedTs: "2023-03-17T01:39:16.897252Z",
      botIdleSinceTs: "2023-03-17T01:32:51.498120Z",
      serverVersions: ["7078-115fec4"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7078-115fec4"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "610605a76664cd11",
      duration: 260.94882,
      performanceStats: {
        botOverhead: 5.514901,
        namedCachesInstall: {
          duration: 0.00054073334,
        },
        namedCachesUninstall: {
          duration: 0.058762312,
        },
        cleanup: {
          duration: 0.8467729,
        },
        cacheTrim: {
          duration: 0.0007419586,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.7180693,
        },
      },
      botVersion:
        "ddf0f9f11cdd69eb0e93336b2ebef48350a61a07a99b29582f7f6cde49c5274e",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "Eilcyxhv5aw4AmUKwgp3m9JpPGq3xlVkb4_mWf0zpcAC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "mZx_7LvYy-gsu4GKwUyBD1z_BjZ0AL8Vq09Zic3WuFsC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "w1lf8GGQpo2pD_w8VxFX6JXRNxVcFNhg_gmYwvOgOdkC",
          },
        ],
      },
      name: "Flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-17T01:30:30.899730Z",
      startedTs: "2023-03-17T01:31:10.013564Z",
      modifiedTs: "2023-03-17T01:31:24.764774Z",
      runId: "61060222889a4811",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0010913812],
      completedTs: "2023-03-17T01:31:24.764774Z",
      botIdleSinceTs: "2023-03-17T01:22:34.587944Z",
      failure: true,
      state: "COMPLETED",
      serverVersions: ["7078-115fec4"],
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7078-115fec4"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "61060222889a4811",
      duration: 2.071994,
      performanceStats: {
        botOverhead: 5.801302,
        namedCachesInstall: {
          duration: 0.00053071976,
        },
        namedCachesUninstall: {
          duration: 0.05954647,
        },
        cleanup: {
          duration: 0.35815,
        },
        cacheTrim: {
          duration: 0.0007560253,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 4.3906374,
        },
      },
      exitCode: 1,
      botVersion:
        "ddf0f9f11cdd69eb0e93336b2ebef48350a61a07a99b29582f7f6cde49c5274e",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "Eilcyxhv5aw4AmUKwgp3m9JpPGq3xlVkb4_mWf0zpcAC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "mZx_7LvYy-gsu4GKwUyBD1z_BjZ0AL8Vq09Zic3WuFsC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "w1lf8GGQpo2pD_w8VxFX6JXRNxVcFNhg_gmYwvOgOdkC",
          },
        ],
      },
      name: "Flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-17T01:20:02.787732Z",
      startedTs: "2023-03-17T01:20:52.613916Z",
      modifiedTs: "2023-03-17T01:21:07.793119Z",
      runId: "6105f88d0051c311",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0011250825],
      completedTs: "2023-03-17T01:21:07.793119Z",
      botIdleSinceTs: "2023-03-16T23:02:04.254651Z",
      failure: true,
      state: "COMPLETED",
      serverVersions: ["7078-115fec4"],
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7078-115fec4"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "6105f88d0051c311",
      duration: 0.005061865,
      performanceStats: {
        botOverhead: 8.272398,
        namedCachesInstall: {
          duration: 0.020037413,
        },
        namedCachesUninstall: {
          duration: 0.50399184,
        },
        cleanup: {
          duration: 0.33834958,
        },
        cacheTrim: {
          duration: 0.00074124336,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 6.435299,
        },
      },
      exitCode: 1,
      botVersion:
        "ddf0f9f11cdd69eb0e93336b2ebef48350a61a07a99b29582f7f6cde49c5274e",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "Eilcyxhv5aw4AmUKwgp3m9JpPGq3xlVkb4_mWf0zpcAC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "mZx_7LvYy-gsu4GKwUyBD1z_BjZ0AL8Vq09Zic3WuFsC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "w1lf8GGQpo2pD_w8VxFX6JXRNxVcFNhg_gmYwvOgOdkC",
          },
        ],
      },
      name: "flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-14T18:52:09.651440Z",
      startedTs: "2023-03-14T18:52:43.227526Z",
      modifiedTs: "2023-03-14T18:59:49.606881Z",
      runId: "60fa48b660a3db11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0325082],
      completedTs: "2023-03-14T18:59:49.606881Z",
      botIdleSinceTs: "2023-03-14T18:44:23.184322Z",
      serverVersions: ["7076-0f93ee5"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7076-0f93ee5"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60fa48b660a3db11",
      duration: 409.87146,
      performanceStats: {
        botOverhead: 8.987192,
        namedCachesInstall: {
          duration: 0.0005800724,
        },
        namedCachesUninstall: {
          duration: 0.32833242,
        },
        cleanup: {
          duration: 1.1984603,
        },
        cacheTrim: {
          duration: 0.0008199215,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 6.299777,
        },
      },
      botVersion:
        "f46540632e20edc29d4e08a07b18c6ff974af71e7ab03dc0865cd449681acd9b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post task for flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-01T01:18:58.971643Z",
      startedTs: "2023-03-01T01:34:19.304045Z",
      modifiedTs: "2023-03-01T01:34:39.660601Z",
      runId: "60b391d3a1346111",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0015303487],
      completedTs: "2023-03-01T01:34:39.660601Z",
      botIdleSinceTs: "2023-03-01T01:34:19.304045Z",
      serverVersions: ["7048-a10855b"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b391d3a1346111",
      duration: 0.0051050186,
      performanceStats: {
        botOverhead: 5.667677,
        namedCachesInstall: {
          duration: 0.00054073334,
        },
        namedCachesUninstall: {
          duration: 0.15827274,
        },
        cleanup: {
          duration: 0.3308301,
        },
        cacheTrim: {
          duration: 0.0007555485,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 4.21672,
        },
      },
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post task for flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-01T01:18:59.232924Z",
      startedTs: "2023-03-01T01:32:26.355266Z",
      modifiedTs: "2023-03-01T01:32:36.660143Z",
      runId: "60b391d4ad923a11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0007585545],
      completedTs: "2023-03-01T01:32:36.660143Z",
      botIdleSinceTs: "2023-03-01T01:31:17.553838Z",
      serverVersions: ["7048-a10855b"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b391d4ad923a11",
      duration: 0.005430937,
      performanceStats: {
        botOverhead: 5.2841644,
        namedCachesInstall: {
          duration: 0.00054883957,
        },
        namedCachesUninstall: {
          duration: 0.15985966,
        },
        cleanup: {
          duration: 0.334625,
        },
        cacheTrim: {
          duration: 0.0007596016,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.8170602,
        },
      },
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-01T01:18:58.717692Z",
      startedTs: "2023-03-01T01:19:46.893913Z",
      modifiedTs: "2023-03-01T01:24:02.415656Z",
      runId: "60b391d2a3fbd111",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.01949766],
      completedTs: "2023-03-01T01:24:02.415656Z",
      botIdleSinceTs: "2023-03-01T01:13:54.038511Z",
      serverVersions: ["7048-a10855b"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b391d2a3fbd111",
      duration: 242.54158,
      performanceStats: {
        botOverhead: 5.8705587,
        namedCachesInstall: {
          duration: 0.00052571297,
        },
        namedCachesUninstall: {
          duration: 0.16452408,
        },
        cleanup: {
          duration: 0.8353474,
        },
        cacheTrim: {
          duration: 0.000756979,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.865362,
        },
      },
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post task for flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-01T00:57:13.823350Z",
      startedTs: "2023-03-01T01:12:06.334346Z",
      modifiedTs: "2023-03-01T01:12:27.375719Z",
      runId: "60b37de964696311",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0015816423],
      completedTs: "2023-03-01T01:12:27.375719Z",
      botIdleSinceTs: "2023-03-01T01:12:06.334346Z",
      serverVersions: ["7048-a10855b"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b37de964696311",
      duration: 0.010287285,
      performanceStats: {
        botOverhead: 5.46626,
        namedCachesInstall: {
          duration: 0.015023708,
        },
        namedCachesUninstall: {
          duration: 0.15897846,
        },
        cleanup: {
          duration: 0.3312955,
        },
        cacheTrim: {
          duration: 0.000800848,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.8127162,
        },
      },
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post task for flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-01T00:57:14.044517Z",
      startedTs: "2023-03-01T01:10:11.212318Z",
      modifiedTs: "2023-03-01T01:10:21.235924Z",
      runId: "60b37dea445f7311",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0007372393],
      completedTs: "2023-03-01T01:10:21.235924Z",
      botIdleSinceTs: "2023-03-01T01:09:02.463023Z",
      serverVersions: ["7048-a10855b"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b37dea445f7311",
      duration: 0.00502491,
      performanceStats: {
        botOverhead: 5.2985206,
        namedCachesInstall: {
          duration: 0.0005297661,
        },
        namedCachesUninstall: {
          duration: 0.15727973,
        },
        cleanup: {
          duration: 0.33203912,
        },
        cacheTrim: {
          duration: 0.0007405281,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.796249,
        },
      },
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-01T00:57:13.578810Z",
      startedTs: "2023-03-01T00:57:24.559444Z",
      modifiedTs: "2023-03-01T01:01:38.796466Z",
      runId: "60b37de873150f11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.019397423],
      completedTs: "2023-03-01T01:01:38.796466Z",
      botIdleSinceTs: "2023-03-01T00:54:12.168291Z",
      serverVersions: ["7048-a10855b"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b37de873150f11",
      duration: 241.13934,
      performanceStats: {
        botOverhead: 5.8362417,
        namedCachesInstall: {
          duration: 0.0005249977,
        },
        namedCachesUninstall: {
          duration: 0.15926003,
        },
        cleanup: {
          duration: 0.8261883,
        },
        cacheTrim: {
          duration: 0.00074625015,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.9014149,
        },
      },
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post task for flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-01T00:52:29.864858Z",
      startedTs: "2023-03-01T00:52:33.491240Z",
      modifiedTs: "2023-03-01T00:52:46.174341Z",
      runId: "60b3799432210111",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.000942615],
      completedTs: "2023-03-01T00:52:46.174341Z",
      botIdleSinceTs: "2023-03-01T00:45:42.912992Z",
      serverVersions: ["7048-a10855b"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b3799432210111",
      duration: 0.0049538612,
      performanceStats: {
        botOverhead: 5.5040903,
        namedCachesInstall: {
          duration: 0.00056934357,
        },
        namedCachesUninstall: {
          duration: 0.15741634,
        },
        cleanup: {
          duration: 0.33227134,
        },
        cacheTrim: {
          duration: 0.00076007843,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 4.0329924,
        },
      },
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post",
      createdTs: "2023-03-01T00:40:09.024085Z",
      startedTs: "2023-03-01T00:43:58.243383Z",
      modifiedTs: "2023-03-01T00:44:16.639940Z",
      runId: "60b36e465391fc11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0013738476],
      completedTs: "2023-03-01T00:44:16.639940Z",
      botIdleSinceTs: "2023-03-01T00:43:58.243383Z",
      failure: true,
      state: "COMPLETED",
      serverVersions: ["7048-a10855b"],
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b36e465391fc11",
      duration: 0.0050451756,
      performanceStats: {
        botOverhead: 5.1566525,
        namedCachesInstall: {
          duration: 0.0005199909,
        },
        namedCachesUninstall: {
          duration: 0.15639496,
        },
        cleanup: {
          duration: 0.32788873,
        },
        cacheTrim: {
          duration: 0.0007379055,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.7248793,
        },
      },
      exitCode: 127,
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post",
      createdTs: "2023-03-01T00:40:09.474680Z",
      startedTs: "2023-03-01T00:42:13.185269Z",
      modifiedTs: "2023-03-01T00:42:31.425534Z",
      runId: "60b36e480a801511",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0013645361],
      completedTs: "2023-03-01T00:42:31.425534Z",
      botIdleSinceTs: "2023-03-01T00:42:13.185269Z",
      failure: true,
      state: "COMPLETED",
      serverVersions: ["7048-a10855b"],
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b36e480a801511",
      duration: 0.004832506,
      performanceStats: {
        botOverhead: 5.416087,
        namedCachesInstall: {
          duration: 0.0005199909,
        },
        namedCachesUninstall: {
          duration: 0.15892935,
        },
        cleanup: {
          duration: 0.33026052,
        },
        cacheTrim: {
          duration: 0.0008201599,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.9044266,
        },
      },
      exitCode: 127,
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "flash",
      createdTs: "2023-03-01T00:40:08.492348Z",
      startedTs: "2023-03-01T00:40:34.731725Z",
      modifiedTs: "2023-03-01T00:40:47.437226Z",
      runId: "60b36e443fff8911",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.000943724],
      completedTs: "2023-03-01T00:40:47.437226Z",
      botIdleSinceTs: "2023-03-01T00:37:43.677025Z",
      failure: true,
      state: "COMPLETED",
      serverVersions: ["7048-a10855b"],
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b36e443fff8911",
      duration: 0.0052051544,
      performanceStats: {
        botOverhead: 5.4200764,
        namedCachesInstall: {
          duration: 0.00051927567,
        },
        namedCachesUninstall: {
          duration: 0.15778422,
        },
        cleanup: {
          duration: 0.33162522,
        },
        cacheTrim: {
          duration: 0.0007481575,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.7816687,
        },
      },
      exitCode: 127,
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post_task_for_flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-01T00:33:47.048818Z",
      startedTs: "2023-03-01T00:35:58.707085Z",
      modifiedTs: "2023-03-01T00:36:17.068129Z",
      runId: "60b3687230673d11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.001377693],
      completedTs: "2023-03-01T00:36:17.068129Z",
      botIdleSinceTs: "2023-03-01T00:35:58.707085Z",
      serverVersions: ["7048-a10855b"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b3687230673d11",
      duration: 0.00494051,
      performanceStats: {
        botOverhead: 5.199508,
        namedCachesInstall: {
          duration: 0.0005364418,
        },
        namedCachesUninstall: {
          duration: 0.1576066,
        },
        cleanup: {
          duration: 0.32944846,
        },
        cacheTrim: {
          duration: 0.0007677078,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.7608945,
        },
      },
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post_task_for_flash build243-m4--device1 to N2G48C",
      createdTs: "2023-03-01T00:33:47.334678Z",
      startedTs: "2023-03-01T00:34:20.169955Z",
      modifiedTs: "2023-03-01T00:34:32.831178Z",
      runId: "60b3687350d7ee11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0009418799],
      completedTs: "2023-03-01T00:34:32.831178Z",
      botIdleSinceTs: "2023-02-28T23:59:24.078979Z",
      serverVersions: ["7048-a10855b"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7048-a10855b"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b3687350d7ee11",
      duration: 0.0055499077,
      performanceStats: {
        botOverhead: 5.308359,
        namedCachesInstall: {
          duration: 0.0005400181,
        },
        namedCachesUninstall: {
          duration: 0.15641809,
        },
        cleanup: {
          duration: 0.33777952,
        },
        cacheTrim: {
          duration: 0.00084137917,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.8731527,
        },
      },
      botVersion:
        "952572ba9c19250ca689dc8fa7fc25eac149070b1c9d62961e1d5928a3c5df6b",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
        ],
      },
      name: "post_task_for_flash build243-m4--device1 to N2G48C",
      createdTs: "2023-02-28T22:24:50.588016Z",
      startedTs: "2023-02-28T22:25:19.666036Z",
      modifiedTs: "2023-02-28T22:25:31.176130Z",
      runId: "60b2f265a2d5c811",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.00084485573],
      completedTs: "2023-02-28T22:25:31.176130Z",
      botIdleSinceTs: "2023-02-28T22:13:01.829217Z",
      serverVersions: ["7045-6320ccc"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7045-6320ccc"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b2f265a2d5c811",
      duration: 0.0051116943,
      performanceStats: {
        botOverhead: 4.4399605,
        namedCachesInstall: {
          duration: 0.0005290508,
        },
        namedCachesUninstall: {
          duration: 0.15937233,
        },
        cleanup: {
          duration: 0.21362948,
        },
        cacheTrim: {
          duration: 0.0007953644,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.0129,
        },
      },
      botVersion:
        "d88b79970c4cf9c44b2639b2b247ca84df740e95d0c7b398846e4ac997db3f9a",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post_task_for_flash build243-m4--device1 to N2G48C",
      createdTs: "2023-02-28T22:06:51.918804Z",
      startedTs: "2023-02-28T22:11:12.916567Z",
      modifiedTs: "2023-02-28T22:11:33.196533Z",
      runId: "60b2e1f0192f3311",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0015248092],
      completedTs: "2023-02-28T22:11:33.196533Z",
      botIdleSinceTs: "2023-02-28T22:11:12.916567Z",
      serverVersions: ["7045-6320ccc"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7045-6320ccc"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b2e1f0192f3311",
      duration: 0.0050435066,
      performanceStats: {
        botOverhead: 5.3528404,
        namedCachesInstall: {
          duration: 0.0005505085,
        },
        namedCachesUninstall: {
          duration: 0.15840769,
        },
        cleanup: {
          duration: 0.33495188,
        },
        cacheTrim: {
          duration: 0.00075387955,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.8999205,
        },
      },
      botVersion:
        "d88b79970c4cf9c44b2639b2b247ca84df740e95d0c7b398846e4ac997db3f9a",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post_task_for_flash build243-m4--device1 to N2G48C",
      createdTs: "2023-02-28T21:56:52.088259Z",
      startedTs: "2023-02-28T22:09:17.372317Z",
      modifiedTs: "2023-02-28T22:09:27.525961Z",
      runId: "60b2d8c8fdec9711",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0007485492],
      completedTs: "2023-02-28T22:09:27.525961Z",
      botIdleSinceTs: "2023-02-28T22:08:08.403803Z",
      serverVersions: ["7045-6320ccc"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7045-6320ccc"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b2d8c8fdec9711",
      duration: 0.004990101,
      performanceStats: {
        botOverhead: 5.822323,
        namedCachesInstall: {
          duration: 0.0005311966,
        },
        namedCachesUninstall: {
          duration: 0.15885687,
        },
        cleanup: {
          duration: 0.3304472,
        },
        cacheTrim: {
          duration: 0.0007622242,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 4.401671,
        },
      },
      botVersion:
        "d88b79970c4cf9c44b2639b2b247ca84df740e95d0c7b398846e4ac997db3f9a",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "LRQEbP9Zoat2J7ZR73Sv8yN9f0Vsm_PXIWkefjzXjpsC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "KAG-xG992eP-D77rG66FiLrkIRi_wdW9wp2oOOZBPJMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "flash build243-m4--device1 to N2G48C",
      createdTs: "2023-02-28T21:56:51.607248Z",
      startedTs: "2023-02-28T21:56:58.522534Z",
      modifiedTs: "2023-02-28T22:01:25.812717Z",
      runId: "60b2d8c71dc8ed11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.020396313],
      completedTs: "2023-02-28T22:01:25.812717Z",
      botIdleSinceTs: "2023-02-28T21:43:32.269428Z",
      serverVersions: ["7045-6320ccc"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7045-6320ccc"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "60b2d8c71dc8ed11",
      duration: 251.12593,
      performanceStats: {
        botOverhead: 9.224713,
        namedCachesInstall: {
          duration: 0.013688803,
        },
        namedCachesUninstall: {
          duration: 1.2408979,
        },
        cleanup: {
          duration: 0.8224199,
        },
        cacheTrim: {
          duration: 0.0007970333,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 6.1780725,
        },
      },
      botVersion:
        "d88b79970c4cf9c44b2639b2b247ca84df740e95d0c7b398846e4ac997db3f9a",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "JlGFsqGCXe6IPNlXDz7BozHuMNqLePKK02vcQPBUjwUC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
        ],
      },
      name: "hypan/id=build243-m4--device1_pool=chromium.tests",
      createdTs: "2023-02-24T18:19:05.402320Z",
      startedTs: "2023-02-24T18:19:42.664872Z",
      modifiedTs: "2023-02-24T18:21:19.221046Z",
      runId: "609d77f74083ba11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.007342564],
      completedTs: "2023-02-24T18:21:19.221046Z",
      botIdleSinceTs: "2023-02-24T18:09:58.026152Z",
      serverVersions: ["7028-cb1c175"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7028-cb1c175"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "609d77f74083ba11",
      duration: 82.92752,
      performanceStats: {
        botOverhead: 6.1913576,
        namedCachesInstall: {
          duration: 0.0007016659,
        },
        namedCachesUninstall: {
          duration: 0.1563406,
        },
        cleanup: {
          duration: 0.3348112,
        },
        cacheTrim: {
          duration: 0.000739336,
        },
        isolatedUpload: {},
        isolatedDownload: {
          duration: 1.6701624,
          totalBytesItemsHot: "1486588386",
          itemsHot:
            "eJzsdXuwltV1/n7WWnvvd7/X7/5958I5HK6HgHJTElABjfwAFTVefjgmJk0EMeJ0aEOnyTidHkTBIxI9BwFLFCOSEyOEiIRYGfHCRamXEOkEihox2ti0iaaE0BiSSTr7/Q6k0+lf/ad/tO+cP86337XXetbzPGu96v+e/90PFBQxQaAU4H/lf/ifxvXfe4zKW1KKVPM/35IiOROA/ACQwUYVSPF/SsLIr4OgCKAERAQlJD6d8ldZFCsQee4GK9F/QRnjdJEzaIjz0zw/uHlKeVqQT0KKqVkmDx/UZDBGKSLgjwqdSXu6xB97aqbwtwjNtptH1MRF5CF4AB4SFLwDFAmTkhxkfk/In0MxA2SbgAAYAmIPhJuoBit43KejASIZvGBy4HkflAeIZ814cHn2JhGSn2MQCAFifGIPlBS0Yl/VBwUAtNfF+Sx526SI81QIYJSxWqlIgXLYOTKlRDyJRKxE/Jli7VWlJqW5Kjk1AmXCnDcIqVBpsM5ve2GUiKfN5rZQAs5fkGbSinMn+FkClG6OFJEPUYrZiaeHPXXiqQdplYcSDVpUeR/6kyYhOpdJE6ucAmiGJWpyGzA4d2fgNWavAntJoeEGZaczjjjjk6YRmsNBOdf/YSh8Md+BJYq9DWx+rpEbQFGzE+hcRgJrZj2YgHVTWJ3z790DaM1iiST/4YlG0x2+sCENsA2VUpn2tCLxCI1CnihgL7ifQGJFmohJaQ2R3GDWZ+DcHHkfJrf46V6l2Vzu8tNmyxGonFZIk1sSz5l/Z420eOS+Xj6WNh8Hvy1YcuiKAs3eJjp3PitS2u+H5gQyvGYmR8T5DeWr6ByN94Dk05FfZ9ucHitM4skOvWLGu1IAC6HQ47YUc+4UPwZMg01RpMhGHpoQNelkraACbxjxkWbwXLyVlSE4FvbqEYtlEwvDaCLj/Wh9WmsSreDrkCTEpCFVBkUQZWPPoQliZbwp86JWk2FmHZKyyjYbpwDstyIDOiLWfidocMDGiLLOOpSEoWIqIVJB6vk3SufrjY0KGDCCxHdPBVFRyecJGbEoziTWOSkBEuPpQJx7XqxvUCuyWicGua5hvolMvhWI87GnxGbMYASKKPXU0BBSSov201eAaI6sTkAhjOLIz6MVUWyJHHEc6lALtApJiZJ8jyhEEGMUQqWVpKyEvTUZyikbmFhrirThRAeBIhsExJYsZw4NSEgaBhwp4YA0cUKpZo59KutI5WwHBQs2BsqwFg4DxYGCE6+nIa1I60SHIIkoCwoRs4WIdMOYBBUqhpmfZytFNAqWKDWKAsp0EJFxRto4IujIOya2yGI4ScKRWiumelpsCwEpulgZxCqE1QUOWIgp1ZJ/6ywCqgVpi18hZc0AlyNOQGdlhuMQVEasSRBkMAap6NASWeZUBan47UdxhkbLGBZlTBrBZk64rEmh7JUzoTWtmWdxtGHDpQ5QbCRp94sx0qpUdpnzMBJVYWFdcbAKnsAsTS0HBrrNWJRi70uTBEFIDpopSMJOilGpVb2nIieBBqQMxSRGtCsnkjRM2TlJhZxITCGPiiPNVEyHUsS6RbKiVlQothhtQ51VpAqxE/MR1C4QQT1lXdMxFWODsF6BzYqu5K1RZYzSWcis2yT23wZtSzYI1Mc0qbLUAXRZM75URjAkCXWlTMhUDWVVM6lOjC3XKTaJ/7YMR9wR2JKEWpeQgCPnEl1gzYUkC+JhjThfvqHDUEmLIs52iLPdLeKsVMtwpqg8Fc6ZlpgKPrbDz6NuKeqIRkTUWskaHQ0RrhfSdEh7tzJDyPh9m7VKaxzYdpiyTisS+W+OMeGkodWUqR216SrhrNAlsXaj1eRh0KhTe1ZrtLWcAxsBQZnH1XVahrJVZUyjtayjzM0sN7psNN1OrGVRUikMTdMkdH5jJqV6ag0KsPWy/nhH0dk2CgN3vUvLlS7W3ZkkHZq7E6vTdiRhdEFnVo1cIYldNbuIk7Oj1kbVtIcjK+PcNJk3vNAonDXWOddxfrVrXnBBobTIjMhitGbBLS2oF8tXWu3mTLA1+0XbhcL8UqHSSWZ4IUzah8f0uU6yUVC0sbFVHVt1XnswpHTxnEvDZGzddXXWS9eY9nGzLAKTVQKMHDFG6mE299y0W5s2N6xrqgx3FzYaadWdmxUvd6rR6vS1enoHd3Yj6Zzn1Kdi6rDmWktxNL7StQITRqd78W3MGnE5Xzi1NKR7JvHZC4qFESOnFXk9klGLcV7xT00btfHMlpsmXnZBuOisOTqR8+bUjbq1Y+nUeEZMZ0/Rv8S00XTFNcM77LmXVaUHcXFMOqEaF9eh0TkcekFyF4otfZjcvjiYMmrJDbVFxc4lZlbhfgztKt3maq2fTCaraLhZh7SQXTDznF5sxzmb8IUonDCj3rJ4WOHmsab7ebhFi66ZQWtQ7/oGalMWtt7yqaxqA62G7sHsZbQVwQrMnWDmJ7N3YT2Ce9A2px+fOHvp1TPduaNb6//v2s7CmGHjr45exIJqYTmGlkdeL4fh9uCS9nmVWbdeegBjJk2bWu3Fwn7MCLdj5I2fv7KN9Kdrr6IST5ry9xh1GEbNrVlT24Srxtb/uk3TAEpTWsrfoLlzC1/8jCw5gnDudafQLccQjLsX8QBntU8+Q43s/1c+O30vrcTF09aiD+kA3sEHGNF5Atfd+G3qmnjF2K/hL1v+jU9hA+7HUziCBf2IPn8/rrtpftstq3FbbRt+Q9ePHfZ1HODhwUnaL7U/m37lbnoLpQHaQsGfTNnDA3jOuNqv8QSeoq+8jL/BWtyOHahsQn3pz3DLR3gM4QqtO4+ja94eTLgXSxrTHsDbdIJf4s9V/vwgrmpsksfxfR6g41Q+IbXtfNlKmfZdLF2OXrniZTyLi1+l5fpDbJZldA/to3jchuAXMvMR+kTwLfd9/NUH5uaHcBBv0HHs4Nk3zDwku+g9fo7u4h9T/CNskUP8IE/rsZ9+jXpxH5Y5/U3+Hh1F+gv8o90lO7jHVJbc9vHH9Qb9vsYlq+w6+cpOXLXgazhqZv0AT9JPw4NsthWG/Aj9wZe+hZ0YiK6+YY2+k+7TrweH9XKctKfKr9JWvQ5PRC/ipvfNq3Qyu/X3ydv2AfTi7fBdeo2fp330Iq+OjtA/xTujp+33ss1y1D0afDDkXbeZv8pr7fm/lX+2jyQ/L+IdrMMu2VLYQ7PX8JzjLd/h3aXfdGzXd9IzvCl9E0vXTO5LVjVex/ZwNe127834lX1S350sy553z5YfqB/J3iirvR3fpTeDhStK60Y/PPH9xpvVJ4duDF7puMu+3vZhtPCV8K1s29DlF239wo7KT4fsj19oeXn+xkkbJx8eszfd+uU7Jj7X8Yy+fQ0eS3/eNdDZU/3BhccL6niwatKKadvnb6hsqT1y8YbunpF/27bzyytnP1FfufChizaWj8q9ozdlKz/2y/Nf6ep/FP/gju7jA/14NNg86uS1h/4OfeG/Ln7Cbf/s4bFbs/smreaHd/Gx0hr72Gf2b8Pu6KWd6P01Vt3cvwndh248sRYb12D9euq9A68do4OP0wtTPhr//PSP1so7+2nzrH2ncPde1/9D7O6lY7/Cw4v71vADT9Lq3fyTl+irR5K1vcFv3wuendrTb088xScP1n9yp1v2HXrtD/jhW/TO76sf7sPP3pN/2d9x7Nj0b26N+7bc1LPzkreXxYdeHq1+t6q07QV6etf4Hz8VPXi4+PW7vtTTc+nv1svAPX3YcWDx7X091Pf0Xxy49/IDD0187A9nPfjoHdj25tXL7j7l3nhuy8h3e9dNOv5Cr9y3Y/ewfw8AAP//YIb1Zw==",
          numItemsHot: "4236",
        },
        packageInstallation: {
          duration: 3.031407,
        },
      },
      botVersion:
        "c6b66226bd8663a5f774e79155e617d48ef5622af2068153c4dc9c0a69bb2ed1",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "JlGFsqGCXe6IPNlXDz7BozHuMNqLePKK02vcQPBUjwUC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "f8qj6mQD7yFek-FON_BpqV43z3qZTDndncBNlUf30ZMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "post_task_for_flash build243-m4--device1 to N2G48C",
      createdTs: "2023-02-24T17:55:35.280438Z",
      startedTs: "2023-02-24T18:08:04.022937Z",
      modifiedTs: "2023-02-24T18:08:13.730463Z",
      runId: "609d627327a3fa11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.0007075232],
      completedTs: "2023-02-24T18:08:13.730463Z",
      botIdleSinceTs: "2023-02-24T18:07:55.063874Z",
      serverVersions: ["7028-cb1c175"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7028-cb1c175"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "609d627327a3fa11",
      duration: 0.00517416,
      performanceStats: {
        botOverhead: 5.1697197,
        namedCachesInstall: {
          duration: 0.0005259514,
        },
        namedCachesUninstall: {
          duration: 0.14943147,
        },
        cleanup: {
          duration: 0.32117963,
        },
        cacheTrim: {
          duration: 0.0007302761,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 3.7368703,
        },
      },
      botVersion:
        "c6b66226bd8663a5f774e79155e617d48ef5622af2068153c4dc9c0a69bb2ed1",
    },
    {
      cipdPins: {
        clientPackage: {
          packageName: "infra/tools/cipd/linux-amd64",
          version: "JlGFsqGCXe6IPNlXDz7BozHuMNqLePKK02vcQPBUjwUC",
        },
        packages: [
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci-auth/linux-amd64",
            version: "eqKjEPSBoP9mXn-E1p26E8HFcthl-MysoVL3YNhdq3MC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/logdog/butler/linux-amd64",
            version: "h3gC8cYK0hcjt0sctAY5vDiQAtUJV_EHkALrHmjmm4kC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython-native/linux-amd64",
            version: "bPTEXkdvGpMLPG77nWXgIExbSJ1q5YCYI4uIJqdL100C",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/luci/vpython/linux-amd64",
            version: "fRdGWFOP-7QaYMB4zTECXQmOzvJhhsRT86V0i_XjHUEC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/rdb/linux-amd64",
            version: "2nQRu8urImzYe2vp27NQ_2Zb_Vu8fCf4g_iI2l3-a0sC",
          },
          {
            path: ".task_template_packages",
            packageName: "infra/tools/result_adapter/linux-amd64",
            version: "8-QdBU1yrb9EQv9t_PtsLZXLy1RkoGCGYzZNKlBt4YcC",
          },
          {
            path: ".task_template_packages/cpython",
            packageName: "infra/3pp/tools/cpython/linux-amd64",
            version: "Xcoqj3Hhx627RnXvJ6XRlqY0JHKSiWGImMYjf5sJSW0C",
          },
          {
            path: ".task_template_packages/cpython3",
            packageName: "infra/3pp/tools/cpython3/linux-amd64",
            version: "PBnbbyurQjsVK0F3kFfFisXJfY7NYP5k477Bzr4sRFMC",
          },
          {
            path: "cipd_devil",
            packageName:
              "infra/3pp/chromium/third_party/catapult/devil/linux-amd64",
            version: "f8qj6mQD7yFek-FON_BpqV43z3qZTDndncBNlUf30ZMC",
          },
          {
            path: "cipd_gsutil",
            packageName: "infra/3pp/tools/gsutil",
            version: "kdsM4E1ZpwSDo9HgvkaPgSGJrfywNPwhAVH_1AKsAIQC",
          },
        ],
      },
      name: "flash build243-m4--device1 to N2G48C",
      createdTs: "2023-02-24T17:55:34.864346Z",
      startedTs: "2023-02-24T17:56:25.590101Z",
      modifiedTs: "2023-02-24T18:00:51.580372Z",
      runId: "609d62715577ee11",
      botLogsCloudProject: "chrome-infra-logs",
      botId: "build243-m4--device1",
      costsUsd: [0.02028872],
      completedTs: "2023-02-24T18:00:51.580372Z",
      botIdleSinceTs: "2023-02-24T16:24:07.206955Z",
      serverVersions: ["7028-cb1c175"],
      state: "COMPLETED",
      botDimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["task_template_vpython_cache"],
          key: "caches",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7028-cb1c175"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      taskId: "609d62715577ee11",
      duration: 251.09743,
      performanceStats: {
        botOverhead: 9.770593,
        namedCachesInstall: {
          duration: 0.0005097389,
        },
        namedCachesUninstall: {
          duration: 0.19967628,
        },
        cleanup: {
          duration: 0.8341539,
        },
        cacheTrim: {
          duration: 0.00082850456,
        },
        isolatedUpload: {},
        isolatedDownload: {},
        packageInstallation: {
          duration: 7.555723,
        },
      },
      botVersion:
        "c6b66226bd8663a5f774e79155e617d48ef5622af2068153c4dc9c0a69bb2ed1",
    },
  ],
  now: "2023-05-25T16:50:21.387922Z",
};

export const eventsMap = {
  // Came from a Skia GPU bot (build16-a9)
  SkiaGPU: [
    {
      externalIp: "74.125.248.106",
      taskId: "12345",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7205-f6cf7ad"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_task",
      ts: "2023-05-31T19:06:02.375921Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27517269422743057,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":1037896,"health":2,"level":81,"power":["AC"],"status":2,"temperature":230,"voltage":4309},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1440000","governor":"interactive"},"disk":{"cache":{"free_mb":25316.8,"size_mb":25483.1},"data":{"free_mb":25316.8,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1018952,"buffers":13284,"cached":897024,"free":108644,"total":1858352,"used":839400},"other_packages":[],"port_path":"3/86","processes":411,"state":"available","temp":{"battery":23.0,"bms":23.0,"pa_therm0":29.0,"pm8994_tz":36.593,"tsens_tz_sensor0":36.0,"tsens_tz_sensor1":41.0,"tsens_tz_sensor10":47.0,"tsens_tz_sensor11":38.0,"tsens_tz_sensor12":37.0,"tsens_tz_sensor13":48.0,"tsens_tz_sensor14":44.0,"tsens_tz_sensor2":37.0,"tsens_tz_sensor3":38.0,"tsens_tz_sensor4":36.0,"tsens_tz_sensor5":38.0,"tsens_tz_sensor7":42.0,"tsens_tz_sensor9":43.0},"uptime":55.24}},"disks":{"/mmutex":{"free_mb":427081.1,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10930,"sleep_streak":149,"ssd":[],"started_ts":1685549025,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":46.0},"uptime":13107,"user":"chrome-bot"}',
      version:
        "e962671e3bc53f7740f1ceadd04974a3ce94f0e5624e8544770246b5ebf2c46e",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7205-f6cf7ad"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_log",
      ts: "2023-05-31T19:04:55.100277Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27517269422743057,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-44250,"health":2,"level":80,"power":[],"status":3,"temperature":220,"voltage":4049},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25316.8,"size_mb":25483.1},"data":{"free_mb":25316.8,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1063976,"buffers":27356,"cached":941064,"free":95556,"total":1858352,"used":794376},"other_packages":[],"port_path":"3/68","processes":383,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.655,"tsens_tz_sensor0":24.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":25.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":10838.4}},"disks":{"/mmutex":{"free_mb":427083.6,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10852,"sleep_streak":148,"ssd":[],"started_ts":1685549025,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":34.0},"uptime":13029,"user":"chrome-bot"}',
      version:
        "e962671e3bc53f7740f1ceadd04974a3ce94f0e5624e8544770246b5ebf2c46e",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Rebooting device because max uptime exceeded during idle",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7205-f6cf7ad"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-31T16:05:17.810816Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27517269422743057,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":597530,"health":2,"level":92,"power":["AC"],"status":2,"temperature":230,"voltage":4354},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.0,"size_mb":25483.1},"data":{"free_mb":25317.0,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1003572,"buffers":16568,"cached":952804,"free":34200,"total":1858352,"used":854780},"other_packages":[],"port_path":"3/68","processes":403,"state":"available","temp":{"battery":23.0,"bms":23.0,"pa_therm0":28.0,"pm8994_tz":32.522,"tsens_tz_sensor0":30.0,"tsens_tz_sensor1":32.0,"tsens_tz_sensor10":33.0,"tsens_tz_sensor11":31.0,"tsens_tz_sensor12":31.0,"tsens_tz_sensor13":32.0,"tsens_tz_sensor14":32.0,"tsens_tz_sensor2":31.0,"tsens_tz_sensor3":31.0,"tsens_tz_sensor4":30.0,"tsens_tz_sensor5":31.0,"tsens_tz_sensor7":32.0,"tsens_tz_sensor9":33.0},"uptime":61.39}},"disks":{"/mmutex":{"free_mb":427117.0,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":77,"sleep_streak":0,"ssd":[],"started_ts":1685549025,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":43.0},"uptime":2254,"user":"chrome-bot"}',
      version:
        "e962671e3bc53f7740f1ceadd04974a3ce94f0e5624e8544770246b5ebf2c46e",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["4"],
          key: "cores",
        },
        {
          value: ["x86", "x86-64", "x86-64-E3-1220_v3", "x86-64-avx2"],
          key: "cpu",
        },
        {
          value: ["powersave"],
          key: "cpu_governor",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["none"],
          key: "gpu",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["1"],
          key: "kvm",
        },
        {
          value: ["n1-highmem-4"],
          key: "machine_type",
        },
        {
          value: [
            "Linux",
            "Ubuntu",
            "Ubuntu-18",
            "Ubuntu-18.04",
            "Ubuntu-18.04.6",
          ],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7205-f6cf7ad"],
          key: "server_version",
        },
        {
          value: ["0"],
          key: "ssd",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_connected",
      ts: "2023-05-31T16:03:45.623335Z",
      state:
        '{"audio":null,"bot_group_cfg_version":null,"cost_usd_hour":0.27517269422743057,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","disks":{"/mmutex":{"free_mb":427119.0,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["dejavu","lato"]},"gpu":null,"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":1,"sleep_streak":0,"ssd":[],"started_ts":1685549025,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":39.0},"uptime":2177,"user":"chrome-bot"}',
      version:
        "e962671e3bc53f7740f1ceadd04974a3ce94f0e5624e8544770246b5ebf2c46e",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7204-983174a"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_shutdown",
      ts: "2023-05-31T16:03:41.285841Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751731228298611,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-42724,"health":2,"level":91,"power":[],"status":3,"temperature":220,"voltage":4187},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.0,"size_mb":25483.1},"data":{"free_mb":25317.0,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1045132,"buffers":18096,"cached":904596,"free":122440,"total":1858352,"used":813220},"other_packages":[],"port_path":"3/54","processes":388,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.553,"tsens_tz_sensor0":24.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":26.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":1351.93}},"disks":{"/mmutex":{"free_mb":427119.5,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":1366,"sleep_streak":22,"ssd":[],"started_ts":1685547637,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":34.0},"uptime":2154,"user":"chrome-bot"}',
      version:
        "f9d34dcc2b6f1ae4de49581747c08b4facc400f3cd0344980ffe0add45c7983e",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message:
        "About to restart: Updating to e962671e3bc53f7740f1ceadd04974a3ce94f0e5624e8544770246b5ebf2c46e",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7204-983174a"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_update",
      ts: "2023-05-31T16:03:39.790993Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751731228298611,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-42724,"health":2,"level":91,"power":[],"status":3,"temperature":220,"voltage":4187},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.0,"size_mb":25483.1},"data":{"free_mb":25317.0,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1045132,"buffers":18096,"cached":904596,"free":122440,"total":1858352,"used":813220},"other_packages":[],"port_path":"3/54","processes":388,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.553,"tsens_tz_sensor0":24.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":26.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":1351.93}},"disks":{"/mmutex":{"free_mb":427119.5,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":1366,"sleep_streak":22,"ssd":[],"started_ts":1685547637,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":34.0},"uptime":2154,"user":"chrome-bot"}',
      version:
        "f9d34dcc2b6f1ae4de49581747c08b4facc400f3cd0344980ffe0add45c7983e",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7204-983174a"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-31T15:42:12.397659Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751731228298611,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":721278,"health":2,"level":84,"power":["AC"],"status":2,"temperature":237,"voltage":4311},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25316.9,"size_mb":25483.1},"data":{"free_mb":25316.9,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1003816,"buffers":16980,"cached":950504,"free":36332,"total":1858352,"used":854536},"other_packages":[],"port_path":"3/54","processes":403,"state":"available","temp":{"battery":24.2,"bms":24.2,"pa_therm0":28.0,"pm8994_tz":33.796,"tsens_tz_sensor0":31.0,"tsens_tz_sensor1":32.0,"tsens_tz_sensor10":33.0,"tsens_tz_sensor11":31.0,"tsens_tz_sensor12":31.0,"tsens_tz_sensor13":33.0,"tsens_tz_sensor14":33.0,"tsens_tz_sensor2":31.0,"tsens_tz_sensor3":31.0,"tsens_tz_sensor4":31.0,"tsens_tz_sensor5":31.0,"tsens_tz_sensor7":33.0,"tsens_tz_sensor9":33.0},"uptime":64.24}},"disks":{"/mmutex":{"free_mb":427124.5,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":78,"sleep_streak":0,"ssd":[],"started_ts":1685547637,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":42.0},"uptime":866,"user":"chrome-bot"}',
      version:
        "f9d34dcc2b6f1ae4de49581747c08b4facc400f3cd0344980ffe0add45c7983e",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["4"],
          key: "cores",
        },
        {
          value: ["x86", "x86-64", "x86-64-E3-1220_v3", "x86-64-avx2"],
          key: "cpu",
        },
        {
          value: ["powersave"],
          key: "cpu_governor",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["none"],
          key: "gpu",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["1"],
          key: "kvm",
        },
        {
          value: ["n1-highmem-4"],
          key: "machine_type",
        },
        {
          value: [
            "Linux",
            "Ubuntu",
            "Ubuntu-18",
            "Ubuntu-18.04",
            "Ubuntu-18.04.6",
          ],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7204-983174a"],
          key: "server_version",
        },
        {
          value: ["0"],
          key: "ssd",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_connected",
      ts: "2023-05-31T15:40:37.274838Z",
      state:
        '{"audio":null,"bot_group_cfg_version":null,"cost_usd_hour":0.2751731228298611,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","disks":{"/mmutex":{"free_mb":427127.0,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["dejavu","lato"]},"gpu":null,"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":0,"sleep_streak":0,"ssd":[],"started_ts":1685547637,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":46.0},"uptime":789,"user":"chrome-bot"}',
      version:
        "f9d34dcc2b6f1ae4de49581747c08b4facc400f3cd0344980ffe0add45c7983e",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_shutdown",
      ts: "2023-05-31T15:40:32.990371Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516341688368057,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":749507,"health":2,"level":83,"power":["AC"],"status":2,"temperature":237,"voltage":4354},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.0,"size_mb":25483.1},"data":{"free_mb":25317.0,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1029496,"buffers":16988,"cached":906116,"free":106392,"total":1858352,"used":828856},"other_packages":[],"port_path":"3/31","processes":398,"state":"available","temp":{"battery":23.7,"bms":23.7,"pa_therm0":25.0,"pm8994_tz":29.183,"tsens_tz_sensor0":26.0,"tsens_tz_sensor1":27.0,"tsens_tz_sensor10":27.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":26.0,"tsens_tz_sensor13":27.0,"tsens_tz_sensor14":27.0,"tsens_tz_sensor2":26.0,"tsens_tz_sensor3":26.0,"tsens_tz_sensor4":26.0,"tsens_tz_sensor5":26.0,"tsens_tz_sensor7":27.0,"tsens_tz_sensor9":28.0},"uptime":574.16}},"disks":{"/mmutex":{"free_mb":427121.7,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":584,"sleep_streak":12,"ssd":[],"started_ts":1685547030,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":38.0},"uptime":766,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message:
        "About to restart: Updating to f9d34dcc2b6f1ae4de49581747c08b4facc400f3cd0344980ffe0add45c7983e",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_update",
      ts: "2023-05-31T15:40:31.585962Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516341688368057,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":749507,"health":2,"level":83,"power":["AC"],"status":2,"temperature":237,"voltage":4354},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.0,"size_mb":25483.1},"data":{"free_mb":25317.0,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1029496,"buffers":16988,"cached":906116,"free":106392,"total":1858352,"used":828856},"other_packages":[],"port_path":"3/31","processes":398,"state":"available","temp":{"battery":23.7,"bms":23.7,"pa_therm0":25.0,"pm8994_tz":29.183,"tsens_tz_sensor0":26.0,"tsens_tz_sensor1":27.0,"tsens_tz_sensor10":27.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":26.0,"tsens_tz_sensor13":27.0,"tsens_tz_sensor14":27.0,"tsens_tz_sensor2":26.0,"tsens_tz_sensor3":26.0,"tsens_tz_sensor4":26.0,"tsens_tz_sensor5":26.0,"tsens_tz_sensor7":27.0,"tsens_tz_sensor9":28.0},"uptime":574.16}},"disks":{"/mmutex":{"free_mb":427121.7,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":584,"sleep_streak":12,"ssd":[],"started_ts":1685547030,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":38.0},"uptime":766,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-31T15:32:00.916041Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516341688368057,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":778804,"health":2,"level":77,"power":["AC"],"status":2,"temperature":242,"voltage":4294},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.0,"size_mb":25483.1},"data":{"free_mb":25317.0,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1008220,"buffers":16572,"cached":951948,"free":39700,"total":1858352,"used":850132},"other_packages":[],"port_path":"3/31","processes":405,"state":"available","temp":{"battery":24.2,"bms":24.2,"pa_therm0":29.0,"pm8994_tz":33.648,"tsens_tz_sensor0":31.0,"tsens_tz_sensor1":33.0,"tsens_tz_sensor10":34.0,"tsens_tz_sensor11":32.0,"tsens_tz_sensor12":31.0,"tsens_tz_sensor13":33.0,"tsens_tz_sensor14":33.0,"tsens_tz_sensor2":32.0,"tsens_tz_sensor3":32.0,"tsens_tz_sensor4":31.0,"tsens_tz_sensor5":31.0,"tsens_tz_sensor7":33.0,"tsens_tz_sensor9":33.0},"uptime":62.9}},"disks":{"/mmutex":{"free_mb":427127.4,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":74,"sleep_streak":0,"ssd":[],"started_ts":1685547030,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":48.0},"uptime":256,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["4"],
          key: "cores",
        },
        {
          value: ["x86", "x86-64", "x86-64-E3-1220_v3", "x86-64-avx2"],
          key: "cpu",
        },
        {
          value: ["powersave"],
          key: "cpu_governor",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["none"],
          key: "gpu",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["1"],
          key: "kvm",
        },
        {
          value: ["n1-highmem-4"],
          key: "machine_type",
        },
        {
          value: [
            "Linux",
            "Ubuntu",
            "Ubuntu-18",
            "Ubuntu-18.04",
            "Ubuntu-18.04.6",
          ],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["0"],
          key: "ssd",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_connected",
      ts: "2023-05-31T15:30:31.389161Z",
      state:
        '{"audio":null,"bot_group_cfg_version":null,"cost_usd_hour":0.27516341688368057,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","disks":{"/mmutex":{"free_mb":427129.6,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["dejavu","lato"]},"gpu":null,"hostname":"build243-m4--device1","ip":"172.17.0.8","nb_files_in_temp":1,"pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":1,"sleep_streak":0,"ssd":[],"started_ts":1685547030,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":43.0},"uptime":183,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_missing",
      ts: "2023-05-31T13:20:10.049272Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-44097,"health":2,"level":80,"power":[],"status":3,"temperature":220,"voltage":4056},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"460800","governor":"interactive"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1070396,"buffers":25132,"cached":890904,"free":154360,"total":1858352,"used":787956},"other_packages":[],"port_path":"3/58","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.998,"tsens_tz_sensor0":25.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":30.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":25.0,"tsens_tz_sensor13":29.0,"tsens_tz_sensor14":28.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":25.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":26.0,"tsens_tz_sensor9":27.0},"uptime":10063.68}},"disks":{"/mmutex":{"free_mb":427098.4,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":42565,"sleep_streak":626,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":32.0},"uptime":86563,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_shutdown",
      ts: "2023-05-31T13:10:02.322630Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-44097,"health":2,"level":80,"power":[],"status":3,"temperature":220,"voltage":4056},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"460800","governor":"interactive"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1070396,"buffers":25132,"cached":890904,"free":154360,"total":1858352,"used":787956},"other_packages":[],"port_path":"3/58","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.998,"tsens_tz_sensor0":25.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":30.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":25.0,"tsens_tz_sensor13":29.0,"tsens_tz_sensor14":28.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":25.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":26.0,"tsens_tz_sensor9":27.0},"uptime":10063.68}},"disks":{"/mmutex":{"free_mb":427098.4,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":42565,"sleep_streak":626,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":32.0},"uptime":86563,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Signal was received",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-31T10:23:06.094482Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":1063378,"health":2,"level":80,"power":["AC"],"status":2,"temperature":230,"voltage":4308},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1440000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1008564,"buffers":13456,"cached":954224,"free":40884,"total":1858352,"used":849788},"other_packages":[],"port_path":"3/58","processes":410,"state":"available","temp":{"battery":23.0,"bms":23.0,"pa_therm0":29.0,"pm8994_tz":36.25,"tsens_tz_sensor0":35.0,"tsens_tz_sensor1":37.0,"tsens_tz_sensor10":45.0,"tsens_tz_sensor11":38.0,"tsens_tz_sensor12":37.0,"tsens_tz_sensor13":41.0,"tsens_tz_sensor14":40.0,"tsens_tz_sensor2":36.0,"tsens_tz_sensor3":36.0,"tsens_tz_sensor4":35.0,"tsens_tz_sensor5":35.0,"tsens_tz_sensor7":36.0,"tsens_tz_sensor9":36.0},"uptime":55.49}},"disks":{"/mmutex":{"free_mb":427110.2,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":32557,"sleep_streak":477,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":35.0},"uptime":76555,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_log",
      ts: "2023-05-31T10:21:58.514336Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-81328,"health":2,"level":80,"power":[],"status":3,"temperature":220,"voltage":4048},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"600000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1061740,"buffers":26488,"cached":888772,"free":146480,"total":1858352,"used":796612},"other_packages":[],"port_path":"3/39","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":26.088,"tsens_tz_sensor0":25.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":31.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":26.0,"tsens_tz_sensor13":29.0,"tsens_tz_sensor14":29.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":25.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":26.0,"tsens_tz_sensor9":27.0},"uptime":10839.39}},"disks":{"/mmutex":{"free_mb":427111.8,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":32488,"sleep_streak":476,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":36.0},"uptime":76486,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Rebooting device because max uptime exceeded during idle",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-31T07:22:13.257358Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":997918,"health":2,"level":80,"power":["AC"],"status":2,"temperature":230,"voltage":4297},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1440000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":992956,"buffers":13360,"cached":946208,"free":33388,"total":1858352,"used":865396},"other_packages":[],"port_path":"3/39","processes":412,"state":"available","temp":{"battery":23.0,"bms":23.0,"pa_therm0":29.0,"pm8994_tz":36.248,"tsens_tz_sensor0":36.0,"tsens_tz_sensor1":38.0,"tsens_tz_sensor10":48.0,"tsens_tz_sensor11":38.0,"tsens_tz_sensor12":37.0,"tsens_tz_sensor13":47.0,"tsens_tz_sensor14":43.0,"tsens_tz_sensor2":36.0,"tsens_tz_sensor3":36.0,"tsens_tz_sensor4":35.0,"tsens_tz_sensor5":36.0,"tsens_tz_sensor7":37.0,"tsens_tz_sensor9":37.0},"uptime":55.44}},"disks":{"/mmutex":{"free_mb":427092.0,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":21704,"sleep_streak":313,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":34.0},"uptime":65702,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_log",
      ts: "2023-05-31T07:21:05.899262Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-43487,"health":2,"level":80,"power":[],"status":3,"temperature":220,"voltage":4054},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"960000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.2,"size_mb":25483.1},"data":{"free_mb":25317.2,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1079040,"buffers":27380,"cached":922716,"free":128944,"total":1858352,"used":779312},"other_packages":[],"port_path":"3/20","processes":381,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.896,"tsens_tz_sensor0":24.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":29.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":25.0,"tsens_tz_sensor13":27.0,"tsens_tz_sensor14":27.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":26.0},"uptime":10813.62}},"disks":{"/mmutex":{"free_mb":427093.4,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":21635,"sleep_streak":312,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":31.0},"uptime":65633,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Rebooting device because max uptime exceeded during idle",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-31T04:21:25.827743Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":709987,"health":2,"level":80,"power":["AC"],"status":2,"temperature":222,"voltage":4229},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1440000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.3,"size_mb":25483.1},"data":{"free_mb":25317.3,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":991924,"buffers":20928,"cached":936824,"free":34172,"total":1858352,"used":866428},"other_packages":[],"port_path":"3/20","processes":404,"state":"available","temp":{"battery":23.0,"bms":23.0,"pa_therm0":30.0,"pm8994_tz":37.672,"tsens_tz_sensor0":39.0,"tsens_tz_sensor1":42.0,"tsens_tz_sensor10":49.0,"tsens_tz_sensor11":41.0,"tsens_tz_sensor12":39.0,"tsens_tz_sensor13":46.0,"tsens_tz_sensor14":45.0,"tsens_tz_sensor2":39.0,"tsens_tz_sensor3":40.0,"tsens_tz_sensor4":38.0,"tsens_tz_sensor5":39.0,"tsens_tz_sensor7":45.0,"tsens_tz_sensor9":45.0},"uptime":34.73}},"disks":{"/mmutex":{"free_mb":427094.6,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10857,"sleep_streak":150,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":38.0},"uptime":54855,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_log",
      ts: "2023-05-31T04:20:39.228482Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-43487,"health":2,"level":80,"power":[],"status":3,"temperature":220,"voltage":4050},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.3,"size_mb":25483.1},"data":{"free_mb":25317.3,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1064456,"buffers":27512,"cached":906844,"free":130100,"total":1858352,"used":793896},"other_packages":[],"port_path":"3/11","processes":383,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.356,"tsens_tz_sensor0":23.0,"tsens_tz_sensor1":24.0,"tsens_tz_sensor10":25.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":10794.88}},"disks":{"/mmutex":{"free_mb":427095.6,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10799,"sleep_streak":149,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":35.0},"uptime":54797,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Rebooting device because max uptime exceeded during idle",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-31T01:21:48.412647Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":734858,"health":2,"level":89,"power":["AC"],"status":2,"temperature":230,"voltage":4354},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1007648,"buffers":16880,"cached":954028,"free":36740,"total":1858352,"used":850704},"other_packages":[],"port_path":"3/11","processes":406,"state":"available","temp":{"battery":23.2,"bms":23.2,"pa_therm0":27.0,"pm8994_tz":32.373,"tsens_tz_sensor0":30.0,"tsens_tz_sensor1":31.0,"tsens_tz_sensor10":32.0,"tsens_tz_sensor11":30.0,"tsens_tz_sensor12":30.0,"tsens_tz_sensor13":31.0,"tsens_tz_sensor14":31.0,"tsens_tz_sensor2":30.0,"tsens_tz_sensor3":30.0,"tsens_tz_sensor4":30.0,"tsens_tz_sensor5":30.0,"tsens_tz_sensor7":31.0,"tsens_tz_sensor9":32.0},"uptime":64.2}},"disks":{"/mmutex":{"free_mb":427110.9,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":69,"sleep_streak":0,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":44.0},"uptime":44066,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["4"],
          key: "cores",
        },
        {
          value: ["x86", "x86-64", "x86-64-E3-1220_v3", "x86-64-avx2"],
          key: "cpu",
        },
        {
          value: ["powersave"],
          key: "cpu_governor",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["none"],
          key: "gpu",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["1"],
          key: "kvm",
        },
        {
          value: ["n1-highmem-4"],
          key: "machine_type",
        },
        {
          value: [
            "Linux",
            "Ubuntu",
            "Ubuntu-18",
            "Ubuntu-18.04",
            "Ubuntu-18.04.6",
          ],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["0"],
          key: "ssd",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_connected",
      ts: "2023-05-31T01:20:22.661444Z",
      state:
        '{"audio":null,"bot_group_cfg_version":null,"cost_usd_hour":0.27516557074652775,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","disks":{"/mmutex":{"free_mb":427038.7,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["dejavu","lato"]},"gpu":null,"hostname":"build243-m4--device1","ip":"172.17.0.5","nb_files_in_temp":1,"pid":51,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":1,"sleep_streak":0,"ssd":[],"started_ts":1685496022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":41.0},"uptime":43998,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_shutdown",
      ts: "2023-05-31T01:15:03.423400Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.275171240234375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-42724,"health":2,"level":89,"power":[],"status":3,"temperature":220,"voltage":4131},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"960000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.2,"size_mb":25483.1},"data":{"free_mb":25317.2,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1086620,"buffers":18916,"cached":876484,"free":191220,"total":1858352,"used":771732},"other_packages":[],"port_path":"3/110","processes":387,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.943,"tsens_tz_sensor0":25.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":29.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":25.0,"tsens_tz_sensor13":27.0,"tsens_tz_sensor14":27.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":25.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":26.0,"tsens_tz_sensor9":27.0},"uptime":3165.15}},"disks":{"/mmutex":{"free_mb":427087.9,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":14061,"sleep_streak":201,"ssd":[],"started_ts":1685481621,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":32.0},"uptime":43657,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Signal was received",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-31T00:22:59.300360Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.275171240234375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":1082757,"health":2,"level":82,"power":["AC"],"status":2,"temperature":230,"voltage":4324},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1440000","governor":"interactive"},"disk":{"cache":{"free_mb":25316.9,"size_mb":25483.1},"data":{"free_mb":25316.9,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":995864,"buffers":13360,"cached":949308,"free":33196,"total":1858352,"used":862488},"other_packages":[],"port_path":"3/110","processes":409,"state":"available","temp":{"battery":23.7,"bms":23.7,"pa_therm0":29.0,"pm8994_tz":36.199,"tsens_tz_sensor0":36.0,"tsens_tz_sensor1":40.0,"tsens_tz_sensor10":47.0,"tsens_tz_sensor11":38.0,"tsens_tz_sensor12":36.0,"tsens_tz_sensor13":46.0,"tsens_tz_sensor14":43.0,"tsens_tz_sensor2":37.0,"tsens_tz_sensor3":37.0,"tsens_tz_sensor4":36.0,"tsens_tz_sensor5":36.0,"tsens_tz_sensor7":39.0,"tsens_tz_sensor9":38.0},"uptime":55.42}},"disks":{"/mmutex":{"free_mb":427095.1,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10951,"sleep_streak":154,"ssd":[],"started_ts":1685481621,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":36.0},"uptime":40548,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_log",
      ts: "2023-05-31T00:21:51.774298Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.275171240234375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-47912,"health":2,"level":81,"power":[],"status":3,"temperature":220,"voltage":4062},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1078820,"buffers":27192,"cached":904612,"free":147016,"total":1858352,"used":779532},"other_packages":[],"port_path":"3/86","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.503,"tsens_tz_sensor0":24.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":25.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":10859.47}},"disks":{"/mmutex":{"free_mb":427096.8,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10873,"sleep_streak":153,"ssd":[],"started_ts":1685481621,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":35.0},"uptime":40470,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Rebooting device because max uptime exceeded during idle",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-30T21:21:54.479325Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.275171240234375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":1015771,"health":2,"level":84,"power":["AC"],"status":2,"temperature":230,"voltage":4329},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1000644,"buffers":17996,"cached":947520,"free":35128,"total":1858352,"used":857708},"other_packages":[],"port_path":"3/86","processes":404,"state":"available","temp":{"battery":23.7,"bms":23.7,"pa_therm0":28.0,"pm8994_tz":33.305,"tsens_tz_sensor0":30.0,"tsens_tz_sensor1":32.0,"tsens_tz_sensor10":33.0,"tsens_tz_sensor11":31.0,"tsens_tz_sensor12":31.0,"tsens_tz_sensor13":32.0,"tsens_tz_sensor14":32.0,"tsens_tz_sensor2":31.0,"tsens_tz_sensor3":31.0,"tsens_tz_sensor4":30.0,"tsens_tz_sensor5":31.0,"tsens_tz_sensor7":32.0,"tsens_tz_sensor9":33.0},"uptime":63.85}},"disks":{"/mmutex":{"free_mb":427091.2,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":78,"sleep_streak":0,"ssd":[],"started_ts":1685481621,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":36.0},"uptime":29674,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["4"],
          key: "cores",
        },
        {
          value: ["x86", "x86-64", "x86-64-E3-1220_v3", "x86-64-avx2"],
          key: "cpu",
        },
        {
          value: ["powersave"],
          key: "cpu_governor",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["none"],
          key: "gpu",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["1"],
          key: "kvm",
        },
        {
          value: ["n1-highmem-4"],
          key: "machine_type",
        },
        {
          value: [
            "Linux",
            "Ubuntu",
            "Ubuntu-18",
            "Ubuntu-18.04",
            "Ubuntu-18.04.6",
          ],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7203-d8cd2a3"],
          key: "server_version",
        },
        {
          value: ["0"],
          key: "ssd",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_connected",
      ts: "2023-05-30T21:20:21.242294Z",
      state:
        '{"audio":null,"bot_group_cfg_version":null,"cost_usd_hour":0.275171240234375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","disks":{"/mmutex":{"free_mb":427092.3,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["dejavu","lato"]},"gpu":null,"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":0,"sleep_streak":0,"ssd":[],"started_ts":1685481621,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":35.0},"uptime":29597,"user":"chrome-bot"}',
      version:
        "e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7202-9c1fc39"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_shutdown",
      ts: "2023-05-30T21:20:17.006740Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751721896701389,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-43029,"health":2,"level":84,"power":[],"status":3,"temperature":220,"voltage":4089},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1076780,"buffers":23044,"cached":949508,"free":104228,"total":1858352,"used":781572},"other_packages":[],"port_path":"3/70","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.405,"tsens_tz_sensor0":23.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":25.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":6561.04}},"disks":{"/mmutex":{"free_mb":427092.6,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":6575,"sleep_streak":92,"ssd":[],"started_ts":1685475023,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":33.0},"uptime":29574,"user":"chrome-bot"}',
      version:
        "0b9c1ba08a21781d88894a5ff9eb792d15eba1712f057dcdcafab35d815558ae",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message:
        "About to restart: Updating to e7a8a06da1293c5f8f176f50fdbcf548085cc5ab1f89339ee6ec15b20afc67ab",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7202-9c1fc39"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_update",
      ts: "2023-05-30T21:20:15.361079Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751721896701389,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-43029,"health":2,"level":84,"power":[],"status":3,"temperature":220,"voltage":4089},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.1,"size_mb":25483.1},"data":{"free_mb":25317.1,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1076780,"buffers":23044,"cached":949508,"free":104228,"total":1858352,"used":781572},"other_packages":[],"port_path":"3/70","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.405,"tsens_tz_sensor0":23.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":25.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":6561.04}},"disks":{"/mmutex":{"free_mb":427092.6,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":6575,"sleep_streak":92,"ssd":[],"started_ts":1685475023,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":33.0},"uptime":29574,"user":"chrome-bot"}',
      version:
        "0b9c1ba08a21781d88894a5ff9eb792d15eba1712f057dcdcafab35d815558ae",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7202-9c1fc39"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-30T19:31:58.385956Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751721896701389,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":736079,"health":2,"level":90,"power":["AC"],"status":2,"temperature":230,"voltage":4354},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.3,"size_mb":25483.1},"data":{"free_mb":25317.3,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1010820,"buffers":18812,"cached":955860,"free":36148,"total":1858352,"used":847532},"other_packages":[],"port_path":"3/70","processes":405,"state":"available","temp":{"battery":23.0,"bms":23.0,"pa_therm0":28.0,"pm8994_tz":32.569,"tsens_tz_sensor0":30.0,"tsens_tz_sensor1":31.0,"tsens_tz_sensor10":32.0,"tsens_tz_sensor11":31.0,"tsens_tz_sensor12":30.0,"tsens_tz_sensor13":31.0,"tsens_tz_sensor14":31.0,"tsens_tz_sensor2":30.0,"tsens_tz_sensor3":30.0,"tsens_tz_sensor4":30.0,"tsens_tz_sensor5":30.0,"tsens_tz_sensor7":32.0,"tsens_tz_sensor9":34.0},"uptime":63.85}},"disks":{"/mmutex":{"free_mb":427108.5,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":78,"sleep_streak":0,"ssd":[],"started_ts":1685475023,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":43.0},"uptime":23077,"user":"chrome-bot"}',
      version:
        "0b9c1ba08a21781d88894a5ff9eb792d15eba1712f057dcdcafab35d815558ae",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["4"],
          key: "cores",
        },
        {
          value: ["x86", "x86-64", "x86-64-E3-1220_v3", "x86-64-avx2"],
          key: "cpu",
        },
        {
          value: ["powersave"],
          key: "cpu_governor",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["none"],
          key: "gpu",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["1"],
          key: "kvm",
        },
        {
          value: ["n1-highmem-4"],
          key: "machine_type",
        },
        {
          value: [
            "Linux",
            "Ubuntu",
            "Ubuntu-18",
            "Ubuntu-18.04",
            "Ubuntu-18.04.6",
          ],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7202-9c1fc39"],
          key: "server_version",
        },
        {
          value: ["0"],
          key: "ssd",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_connected",
      ts: "2023-05-30T19:30:23.531220Z",
      state:
        '{"audio":null,"bot_group_cfg_version":null,"cost_usd_hour":0.2751721896701389,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","disks":{"/mmutex":{"free_mb":427109.8,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["dejavu","lato"]},"gpu":null,"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":0,"sleep_streak":0,"ssd":[],"started_ts":1685475023,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":34.0},"uptime":22999,"user":"chrome-bot"}',
      version:
        "0b9c1ba08a21781d88894a5ff9eb792d15eba1712f057dcdcafab35d815558ae",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7200-07ad3da"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_shutdown",
      ts: "2023-05-30T19:30:19.265938Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27517320421006947,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-42724,"health":2,"level":90,"power":[],"status":3,"temperature":220,"voltage":4150},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.3,"size_mb":25483.1},"data":{"free_mb":25317.3,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1087816,"buffers":20236,"cached":898412,"free":169168,"total":1858352,"used":770536},"other_packages":[],"port_path":"3/60","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.653,"tsens_tz_sensor0":24.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":26.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":3434.2}},"disks":{"/mmutex":{"free_mb":427109.9,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":3439,"sleep_streak":49,"ssd":[],"started_ts":1685471562,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":33.0},"uptime":22976,"user":"chrome-bot"}',
      version:
        "79612a697d5737bf8b4cd2edfc28f5a5a2cba37c14ccd03549c11086b3d76dc4",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message:
        "About to restart: Updating to 0b9c1ba08a21781d88894a5ff9eb792d15eba1712f057dcdcafab35d815558ae",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7200-07ad3da"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_update",
      ts: "2023-05-30T19:30:17.587464Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27517320421006947,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-42724,"health":2,"level":90,"power":[],"status":3,"temperature":220,"voltage":4150},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.3,"size_mb":25483.1},"data":{"free_mb":25317.3,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1087816,"buffers":20236,"cached":898412,"free":169168,"total":1858352,"used":770536},"other_packages":[],"port_path":"3/60","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.653,"tsens_tz_sensor0":24.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":26.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":3434.2}},"disks":{"/mmutex":{"free_mb":427109.9,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":3439,"sleep_streak":49,"ssd":[],"started_ts":1685471562,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":33.0},"uptime":22976,"user":"chrome-bot"}',
      version:
        "79612a697d5737bf8b4cd2edfc28f5a5a2cba37c14ccd03549c11086b3d76dc4",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7200-07ad3da"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-30T18:34:05.472987Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.27517320421006947,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":975336,"health":2,"level":82,"power":["AC"],"status":2,"temperature":232,"voltage":4315},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.3,"size_mb":25483.1},"data":{"free_mb":25317.3,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1008388,"buffers":16608,"cached":953124,"free":38656,"total":1858352,"used":849964},"other_packages":[],"port_path":"3/60","processes":405,"state":"available","temp":{"battery":23.2,"bms":23.2,"pa_therm0":28.0,"pm8994_tz":32.619,"tsens_tz_sensor0":30.0,"tsens_tz_sensor1":32.0,"tsens_tz_sensor10":33.0,"tsens_tz_sensor11":31.0,"tsens_tz_sensor12":31.0,"tsens_tz_sensor13":32.0,"tsens_tz_sensor14":32.0,"tsens_tz_sensor2":31.0,"tsens_tz_sensor3":31.0,"tsens_tz_sensor4":30.0,"tsens_tz_sensor5":31.0,"tsens_tz_sensor7":32.0,"tsens_tz_sensor9":33.0},"uptime":61.69}},"disks":{"/mmutex":{"free_mb":427123.8,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":68,"sleep_streak":0,"ssd":[],"started_ts":1685471562,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":42.0},"uptime":19606,"user":"chrome-bot"}',
      version:
        "79612a697d5737bf8b4cd2edfc28f5a5a2cba37c14ccd03549c11086b3d76dc4",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["4"],
          key: "cores",
        },
        {
          value: ["x86", "x86-64", "x86-64-E3-1220_v3", "x86-64-avx2"],
          key: "cpu",
        },
        {
          value: ["powersave"],
          key: "cpu_governor",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["none"],
          key: "gpu",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["1"],
          key: "kvm",
        },
        {
          value: ["n1-highmem-4"],
          key: "machine_type",
        },
        {
          value: [
            "Linux",
            "Ubuntu",
            "Ubuntu-18",
            "Ubuntu-18.04",
            "Ubuntu-18.04.6",
          ],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7200-07ad3da"],
          key: "server_version",
        },
        {
          value: ["0"],
          key: "ssd",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_connected",
      ts: "2023-05-30T18:32:42.369607Z",
      state:
        '{"audio":null,"bot_group_cfg_version":null,"cost_usd_hour":0.27517320421006947,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","disks":{"/mmutex":{"free_mb":427128.4,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["dejavu","lato"]},"gpu":null,"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":1,"sleep_streak":0,"ssd":[],"started_ts":1685471562,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":35.0},"uptime":19538,"user":"chrome-bot"}',
      version:
        "79612a697d5737bf8b4cd2edfc28f5a5a2cba37c14ccd03549c11086b3d76dc4",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_shutdown",
      ts: "2023-05-30T18:32:38.073925Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751654079861111,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-224760,"health":2,"level":82,"power":[],"status":3,"temperature":220,"voltage":4075},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"960000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.4,"size_mb":25483.1},"data":{"free_mb":25317.4,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1074036,"buffers":24456,"cached":901300,"free":148280,"total":1858352,"used":784316},"other_packages":[],"port_path":"3/34","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":26.244,"tsens_tz_sensor0":25.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":30.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":26.0,"tsens_tz_sensor13":29.0,"tsens_tz_sensor14":29.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":25.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":27.0,"tsens_tz_sensor9":27.0},"uptime":8498.7}},"disks":{"/mmutex":{"free_mb":427133.9,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":19320,"sleep_streak":278,"ssd":[],"started_ts":1685452229,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":34.0},"uptime":19525,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message:
        "About to restart: Updating to 79612a697d5737bf8b4cd2edfc28f5a5a2cba37c14ccd03549c11086b3d76dc4",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_update",
      ts: "2023-05-30T18:32:36.616714Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751654079861111,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-224760,"health":2,"level":82,"power":[],"status":3,"temperature":220,"voltage":4075},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"960000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.4,"size_mb":25483.1},"data":{"free_mb":25317.4,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1074036,"buffers":24456,"cached":901300,"free":148280,"total":1858352,"used":784316},"other_packages":[],"port_path":"3/34","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":26.244,"tsens_tz_sensor0":25.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":30.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":26.0,"tsens_tz_sensor13":29.0,"tsens_tz_sensor14":29.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":25.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":27.0,"tsens_tz_sensor9":27.0},"uptime":8498.7}},"disks":{"/mmutex":{"free_mb":427133.9,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":19320,"sleep_streak":278,"ssd":[],"started_ts":1685452229,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":34.0},"uptime":19525,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-30T16:11:53.995656Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751654079861111,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":1057427,"health":2,"level":82,"power":["AC"],"status":2,"temperature":230,"voltage":4320},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1440000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.4,"size_mb":25483.1},"data":{"free_mb":25317.4,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1002340,"buffers":13456,"cached":942992,"free":45892,"total":1858352,"used":856012},"other_packages":[],"port_path":"3/34","processes":410,"state":"available","temp":{"battery":23.7,"bms":23.7,"pa_therm0":29.0,"pm8994_tz":36.201,"tsens_tz_sensor0":35.0,"tsens_tz_sensor1":37.0,"tsens_tz_sensor10":43.0,"tsens_tz_sensor11":38.0,"tsens_tz_sensor12":36.0,"tsens_tz_sensor13":40.0,"tsens_tz_sensor14":40.0,"tsens_tz_sensor2":36.0,"tsens_tz_sensor3":36.0,"tsens_tz_sensor4":35.0,"tsens_tz_sensor5":35.0,"tsens_tz_sensor7":36.0,"tsens_tz_sensor9":37.0},"uptime":55.05}},"disks":{"/mmutex":{"free_mb":427137.0,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10877,"sleep_streak":150,"ssd":[],"started_ts":1685452229,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":40.0},"uptime":11082,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_log",
      ts: "2023-05-30T16:10:45.451456Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751654079861111,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-44097,"health":2,"level":81,"power":[],"status":3,"temperature":220,"voltage":4065},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.5,"size_mb":25483.1},"data":{"free_mb":25317.5,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1067152,"buffers":27376,"cached":916608,"free":123168,"total":1858352,"used":791200},"other_packages":[],"port_path":"3/25","processes":382,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.751,"tsens_tz_sensor0":24.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":26.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":26.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":26.0},"uptime":10793.33}},"disks":{"/mmutex":{"free_mb":427146.3,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10799,"sleep_streak":149,"ssd":[],"started_ts":1685452229,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":33.0},"uptime":11003,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Rebooting device because max uptime exceeded during idle",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-30T13:11:56.872087Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751654079861111,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":1209099,"health":2,"level":81,"power":["AC"],"status":2,"temperature":232,"voltage":4340},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.6,"size_mb":25483.1},"data":{"free_mb":25317.6,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1009252,"buffers":16468,"cached":954800,"free":37984,"total":1858352,"used":849100},"other_packages":[],"port_path":"3/25","processes":405,"state":"available","temp":{"battery":23.2,"bms":23.2,"pa_therm0":27.0,"pm8994_tz":32.03,"tsens_tz_sensor0":29.0,"tsens_tz_sensor1":31.0,"tsens_tz_sensor10":32.0,"tsens_tz_sensor11":30.0,"tsens_tz_sensor12":30.0,"tsens_tz_sensor13":31.0,"tsens_tz_sensor14":31.0,"tsens_tz_sensor2":30.0,"tsens_tz_sensor3":30.0,"tsens_tz_sensor4":29.0,"tsens_tz_sensor5":30.0,"tsens_tz_sensor7":31.0,"tsens_tz_sensor9":32.0},"uptime":65.09}},"disks":{"/mmutex":{"free_mb":427164.5,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":70,"sleep_streak":0,"ssd":[],"started_ts":1685452229,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":48.0},"uptime":274,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["4"],
          key: "cores",
        },
        {
          value: ["x86", "x86-64", "x86-64-E3-1220_v3", "x86-64-avx2"],
          key: "cpu",
        },
        {
          value: ["powersave"],
          key: "cpu_governor",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["none"],
          key: "gpu",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["1"],
          key: "kvm",
        },
        {
          value: ["n1-highmem-4"],
          key: "machine_type",
        },
        {
          value: [
            "Linux",
            "Ubuntu",
            "Ubuntu-18",
            "Ubuntu-18.04",
            "Ubuntu-18.04.6",
          ],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["0"],
          key: "ssd",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_connected",
      ts: "2023-05-30T13:10:29.771366Z",
      state:
        '{"audio":null,"bot_group_cfg_version":null,"cost_usd_hour":0.2751654079861111,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","disks":{"/mmutex":{"free_mb":427166.3,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["dejavu","lato"]},"gpu":null,"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":1,"sleep_streak":0,"ssd":[],"started_ts":1685452229,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":43.0},"uptime":205,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_shutdown",
      ts: "2023-05-30T13:00:02.802537Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751943359375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-46691,"health":2,"level":81,"power":[],"status":3,"temperature":220,"voltage":4065},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"460800","governor":"interactive"},"disk":{"cache":{"free_mb":25317.6,"size_mb":25483.1},"data":{"free_mb":25317.6,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1069752,"buffers":25528,"cached":892436,"free":151788,"total":1858352,"used":788600},"other_packages":[],"port_path":"3/52","processes":385,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":26.143,"tsens_tz_sensor0":25.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":31.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":26.0,"tsens_tz_sensor13":29.0,"tsens_tz_sensor14":29.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":25.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":26.0,"tsens_tz_sensor9":27.0},"uptime":9936.04}},"disks":{"/mmutex":{"free_mb":427152.0,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":42533,"sleep_streak":629,"ssd":[],"started_ts":1685409022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":34.0},"uptime":86523,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Signal was received",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-30T10:14:41.781672Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751943359375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":1079858,"health":2,"level":80,"power":["AC"],"status":2,"temperature":232,"voltage":4313},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1440000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.5,"size_mb":25483.1},"data":{"free_mb":25317.5,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1006992,"buffers":13936,"cached":954472,"free":38584,"total":1858352,"used":851360},"other_packages":[],"port_path":"3/52","processes":409,"state":"available","temp":{"battery":23.2,"bms":23.2,"pa_therm0":29.0,"pm8994_tz":36.74,"tsens_tz_sensor0":36.0,"tsens_tz_sensor1":39.0,"tsens_tz_sensor10":48.0,"tsens_tz_sensor11":38.0,"tsens_tz_sensor12":37.0,"tsens_tz_sensor13":45.0,"tsens_tz_sensor14":43.0,"tsens_tz_sensor2":36.0,"tsens_tz_sensor3":37.0,"tsens_tz_sensor4":35.0,"tsens_tz_sensor5":36.0,"tsens_tz_sensor7":38.0,"tsens_tz_sensor9":38.0},"uptime":55.28}},"disks":{"/mmutex":{"free_mb":427154.2,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":32653,"sleep_streak":481,"ssd":[],"started_ts":1685409022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":43.0},"uptime":76642,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_log",
      ts: "2023-05-30T10:13:34.641558Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751943359375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-45928,"health":2,"level":80,"power":[],"status":3,"temperature":220,"voltage":4056},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1248000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.6,"size_mb":25483.1},"data":{"free_mb":25317.6,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1065688,"buffers":27248,"cached":896236,"free":142204,"total":1858352,"used":792664},"other_packages":[],"port_path":"3/34","processes":386,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":26.094,"tsens_tz_sensor0":25.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":29.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":26.0,"tsens_tz_sensor13":27.0,"tsens_tz_sensor14":27.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":26.0,"tsens_tz_sensor9":26.0},"uptime":10830.72}},"disks":{"/mmutex":{"free_mb":427153.8,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":32584,"sleep_streak":480,"ssd":[],"started_ts":1685409022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":42.0},"uptime":76573,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Rebooting device because max uptime exceeded during idle",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-30T07:13:57.792344Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751943359375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":1034387,"health":2,"level":80,"power":["AC"],"status":2,"temperature":232,"voltage":4300},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1440000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.6,"size_mb":25483.1},"data":{"free_mb":25317.6,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1004956,"buffers":13968,"cached":949032,"free":41956,"total":1858352,"used":853396},"other_packages":[],"port_path":"3/34","processes":412,"state":"available","temp":{"battery":23.2,"bms":23.2,"pa_therm0":29.0,"pm8994_tz":36.397,"tsens_tz_sensor0":36.0,"tsens_tz_sensor1":41.0,"tsens_tz_sensor10":47.0,"tsens_tz_sensor11":38.0,"tsens_tz_sensor12":37.0,"tsens_tz_sensor13":44.0,"tsens_tz_sensor14":42.0,"tsens_tz_sensor2":37.0,"tsens_tz_sensor3":37.0,"tsens_tz_sensor4":36.0,"tsens_tz_sensor5":37.0,"tsens_tz_sensor7":41.0,"tsens_tz_sensor9":41.0},"uptime":55.35}},"disks":{"/mmutex":{"free_mb":427124.0,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":21809,"sleep_streak":319,"ssd":[],"started_ts":1685409022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":36.0},"uptime":65798,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_log",
      ts: "2023-05-30T07:12:50.547319Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751943359375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-44402,"health":2,"level":79,"power":[],"status":3,"temperature":220,"voltage":4027},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"600000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.6,"size_mb":25483.1},"data":{"free_mb":25317.6,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1059900,"buffers":27000,"cached":903520,"free":129380,"total":1858352,"used":798452},"other_packages":[],"port_path":"3/16","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.945,"tsens_tz_sensor0":25.0,"tsens_tz_sensor1":26.0,"tsens_tz_sensor10":29.0,"tsens_tz_sensor11":26.0,"tsens_tz_sensor12":26.0,"tsens_tz_sensor13":27.0,"tsens_tz_sensor14":27.0,"tsens_tz_sensor2":25.0,"tsens_tz_sensor3":25.0,"tsens_tz_sensor4":25.0,"tsens_tz_sensor5":25.0,"tsens_tz_sensor7":26.0,"tsens_tz_sensor9":26.0},"uptime":10862.86}},"disks":{"/mmutex":{"free_mb":427125.8,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":21740,"sleep_streak":318,"ssd":[],"started_ts":1685409022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":37.0},"uptime":65730,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Rebooting device because max uptime exceeded during idle",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-30T04:12:42.397031Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751943359375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":1018670,"health":2,"level":82,"power":["AC"],"status":2,"temperature":230,"voltage":4313},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"1440000","governor":"interactive"},"disk":{"cache":{"free_mb":25317.6,"size_mb":25483.1},"data":{"free_mb":25317.6,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1000060,"buffers":13624,"cached":950104,"free":36332,"total":1858352,"used":858292},"other_packages":[],"port_path":"3/16","processes":408,"state":"available","temp":{"battery":23.0,"bms":23.0,"pa_therm0":29.0,"pm8994_tz":35.169,"tsens_tz_sensor0":34.0,"tsens_tz_sensor1":36.0,"tsens_tz_sensor10":39.0,"tsens_tz_sensor11":36.0,"tsens_tz_sensor12":35.0,"tsens_tz_sensor13":37.0,"tsens_tz_sensor14":37.0,"tsens_tz_sensor2":34.0,"tsens_tz_sensor3":35.0,"tsens_tz_sensor4":34.0,"tsens_tz_sensor5":34.0,"tsens_tz_sensor7":36.0,"tsens_tz_sensor9":36.0},"uptime":55.56}},"disks":{"/mmutex":{"free_mb":427722.0,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10933,"sleep_streak":147,"ssd":[],"started_ts":1685409022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":39.0},"uptime":54923,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_log",
      ts: "2023-05-30T04:11:34.704682Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751943359375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":-43487,"health":2,"level":81,"power":[],"status":3,"temperature":220,"voltage":4066},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.7,"size_mb":25483.1},"data":{"free_mb":25317.7,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1080388,"buffers":26916,"cached":915908,"free":137564,"total":1858352,"used":777964},"other_packages":[],"port_path":"3/117","processes":384,"state":"available","temp":{"battery":22.0,"bms":22.0,"pa_therm0":23.0,"pm8994_tz":25.553,"tsens_tz_sensor0":24.0,"tsens_tz_sensor1":25.0,"tsens_tz_sensor10":25.0,"tsens_tz_sensor11":24.0,"tsens_tz_sensor12":24.0,"tsens_tz_sensor13":25.0,"tsens_tz_sensor14":25.0,"tsens_tz_sensor2":24.0,"tsens_tz_sensor3":24.0,"tsens_tz_sensor4":24.0,"tsens_tz_sensor5":24.0,"tsens_tz_sensor7":25.0,"tsens_tz_sensor9":25.0},"uptime":10850.24}},"disks":{"/mmutex":{"free_mb":427724.6,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":10855,"sleep_streak":146,"ssd":[],"started_ts":1685409022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":39.0},"uptime":54845,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
      message: "Rebooting device because max uptime exceeded during idle",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["1"],
          key: "android_devices",
        },
        {
          value: ["android.py"],
          key: "bot_config",
        },
        {
          value: ["arm64-v8a"],
          key: "device_abi",
        },
        {
          value: ["9.8.79"],
          key: "device_gms_core_version",
        },
        {
          value: ["<=18000"],
          key: "device_max_uid",
        },
        {
          value: ["N", "N2G48C"],
          key: "device_os",
        },
        {
          value: ["google"],
          key: "device_os_flavor",
        },
        {
          value: ["userdebug"],
          key: "device_os_type",
        },
        {
          value: ["7.4.31.L-all"],
          key: "device_playstore_version",
        },
        {
          value: ["bullhead"],
          key: "device_type",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["Android"],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["<30"],
          key: "temp_band",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "request_sleep",
      ts: "2023-05-30T01:11:46.138685Z",
      state:
        '{"audio":null,"bot_config":{"name":"android.py","revision":""},"bot_group_cfg_version":"hash:e5b4db888ef54c","cost_usd_hour":0.2751943359375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","devices":{"01e1e815a28da48d":{"battery":{"current":744166,"health":2,"level":86,"power":["AC"],"status":2,"temperature":242,"voltage":4354},"build":{"board.platform":"msm8992","build.fingerprint":"google/bullhead/bullhead:7.1.2/N2G48C/4104010:userdebug/dev-keys","build.id":"N2G48C","build.product":"bullhead","build.type":"userdebug","build.version.sdk":"25","product.board":"bullhead","product.cpu.abi":"arm64-v8a","product.device":"bullhead"},"cpu":{"cur":"384000","governor":"powersave"},"disk":{"cache":{"free_mb":25317.5,"size_mb":25483.1},"data":{"free_mb":25317.5,"size_mb":25483.1},"system":{"free_mb":249.8,"size_mb":2929.2}},"imei":"353627078855551","ip":[],"max_uid":10096,"mem":{"avail":1011848,"buffers":16668,"cached":959880,"free":35300,"total":1858352,"used":846504},"other_packages":[],"port_path":"3/117","processes":405,"state":"available","temp":{"battery":24.2,"bms":24.2,"pa_therm0":29.0,"pm8994_tz":34.041,"tsens_tz_sensor0":31.0,"tsens_tz_sensor1":33.0,"tsens_tz_sensor10":34.0,"tsens_tz_sensor11":32.0,"tsens_tz_sensor12":32.0,"tsens_tz_sensor13":33.0,"tsens_tz_sensor14":33.0,"tsens_tz_sensor2":32.0,"tsens_tz_sensor3":32.0,"tsens_tz_sensor4":31.0,"tsens_tz_sensor5":31.0,"tsens_tz_sensor7":33.0,"tsens_tz_sensor9":34.0},"uptime":61.83}},"disks":{"/mmutex":{"free_mb":427611.5,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"gpu":null,"host_dimensions":{"cores":["4"],"cpu":["x86","x86-64","x86-64-E3-1220_v3","x86-64-avx2"],"cpu_governor":["powersave"],"gce":["0"],"gpu":["none"],"id":["build243-m4--device1"],"inside_docker":["1","stock"],"kernel":["5.4.0-99-generic"],"kvm":["1"],"machine_type":["n1-highmem-4"],"os":["Linux","Ubuntu","Ubuntu-18","Ubuntu-18.04","Ubuntu-18.04.6"],"python":["3","3.6","3.6.9"],"ssd":["0"]},"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"original_bot_id":"build243-m4--device1","pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":69,"sleep_streak":0,"ssd":[],"started_ts":1685409022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":44.0},"uptime":44058,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
    {
      externalIp: "74.125.248.106",
      dimensions: [
        {
          value: ["4"],
          key: "cores",
        },
        {
          value: ["x86", "x86-64", "x86-64-E3-1220_v3", "x86-64-avx2"],
          key: "cpu",
        },
        {
          value: ["powersave"],
          key: "cpu_governor",
        },
        {
          value: ["0"],
          key: "gce",
        },
        {
          value: ["none"],
          key: "gpu",
        },
        {
          value: ["build243-m4--device1"],
          key: "id",
        },
        {
          value: ["1", "stock"],
          key: "inside_docker",
        },
        {
          value: ["5.4.0-99-generic"],
          key: "kernel",
        },
        {
          value: ["1"],
          key: "kvm",
        },
        {
          value: ["n1-highmem-4"],
          key: "machine_type",
        },
        {
          value: [
            "Linux",
            "Ubuntu",
            "Ubuntu-18",
            "Ubuntu-18.04",
            "Ubuntu-18.04.6",
          ],
          key: "os",
        },
        {
          value: ["chromium.tests"],
          key: "pool",
        },
        {
          value: ["3", "3.6", "3.6.9"],
          key: "python",
        },
        {
          value: ["7195-30c06ed"],
          key: "server_version",
        },
        {
          value: ["0"],
          key: "ssd",
        },
        {
          value: ["us", "us-atl", "us-atl-golo", "us-atl-golo-m4"],
          key: "zone",
        },
      ],
      eventType: "bot_connected",
      ts: "2023-05-30T01:10:22.676532Z",
      state:
        '{"audio":null,"bot_group_cfg_version":null,"cost_usd_hour":0.2751943359375,"cpu_name":"Intel(R) Xeon(R) CPU E3-1220 v3 @ 3.10GHz","cwd":"/b/swarming","disks":{"/mmutex":{"free_mb":427543.9,"size_mb":467464.7}},"docker_host_hostname":"build243-m4.golo.chromium.org","env":{"PATH":"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"},"files":{"/usr/share/fonts/truetype/":["dejavu","lato"]},"gpu":null,"hostname":"build243-m4--device1","ip":"172.17.0.6","nb_files_in_temp":1,"pid":52,"python":{"executable":"/usr/bin/python3","packages":null,"version":"3.6.9 (default, Feb 28 2023, 09:55:20) \\n[GCC 8.4.0]"},"ram":23881,"rbe_instance":null,"running_time":0,"sleep_streak":0,"ssd":[],"started_ts":1685409022,"temp":{"thermal_zone0":27.8,"thermal_zone1":29.8,"thermal_zone2":44.0},"uptime":43990,"user":"chrome-bot"}',
      version:
        "8f730a2e82e73e28788df638575d4d6008dd31727a1534d4a726056d594ca412",
      authenticatedAs: "bot:build243-m4.golo.chromium.org",
    },
  ],
};
