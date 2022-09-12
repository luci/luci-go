var CONFIGS = Object.freeze({
  RESULT_DB: Object.freeze({
    HOST: "{{.ResultDB.Host}}",
  }),
  BUILDBUCKET: Object.freeze({
    HOST: "{{.Buildbucket.Host}}",
  }),
  LUCI_ANALYSIS: Object.freeze({
    HOST: "{{.LuciAnalysis.Host}}",
  }),
});
