var CONFIGS = Object.freeze({
  RESULT_DB: Object.freeze({
    HOST: "{{.ResultDB.Host}}",
  }),
  BUILDBUCKET: Object.freeze({
    HOST: "{{.Buildbucket.Host}}",
  }),
  OAUTH2: Object.freeze({
    CLIENT_ID: "{{.OAuth2.ClientID}}",
  }),
});
