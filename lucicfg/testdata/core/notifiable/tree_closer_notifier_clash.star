luci.tree_closer(
    name = "some name",
    tree_status_host = "some-tree.example.com",
)

luci.notifier(
    name = "some name",
    on_occurrence = ["FAILURE"],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //notifiable/tree_closer_notifier_clash.star: in <toplevel>
#   ...
# Error in add_node: luci.notifiable("some name") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //notifiable/tree_closer_notifier_clash.star: in <toplevel>
#   ...
