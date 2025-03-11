luci.project(
    name = "project",
    milo = "luci-milo.appspot.com",
)

luci.list_view(name = "Some view")
luci.console_view(name = "Some view", repo = "https://some.repo")

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/view_name_clashes.star: in <toplevel>
#   ...
# Error in add_node: luci.milo_view("Some view") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //errors/view_name_clashes.star: in <toplevel>
#   ...
