var CONFIGS = Object.freeze({
  RESULT_DB: Object.freeze({
    HOST: "{{.ResultDB.Host}}",
  }),
  BUILDBUCKET: Object.freeze({
    HOST: "{{.Buildbucket.Host}}",
  }),
  WEETBIX: Object.freeze({
    HOST: "{{.Weetbix.Host}}",
  }),
  LUCI_ANALYSIS: Object.freeze({
    HOST: "{{.LuciAnalysis.Host}}",
  }),
});
