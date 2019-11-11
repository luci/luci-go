# Filegraph

Filegraph is a graph of files in a git repository, where a node is a file
and a weight from 0.0 to 1.0 of edge (A, B) represents how much file B is
relevant to file A. The tool has subcommands to compute relevance between any
two files based on the shortest path, where distance is -log(relevance),
as well as order files by distance from a given set of root files. This can
be used to order tests by relevance to files modified in a CL.

TODO(nodir): elaborate.
