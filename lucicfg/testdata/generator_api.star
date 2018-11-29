load("@proto//luci/config/project_config.proto", config_pb="config")

def gen(ctx):
  ctx.config_set['text.txt'] = 'Just text\n'
  ctx.config_set['json.json'] = to_json({'hello': 'world'}) + '\n'
  ctx.config_set['project.cfg'] = config_pb.ProjectCfg(
      name = 'test',
      access = ['group:all'],
  )
core.generator(impl = gen)

# Expect configs:
#
# === json.json
# {"hello":"world"}
# ===
#
# === project.cfg
# name: "test"
# access: "group:all"
# ===
#
# === text.txt
# Just text
# ===
