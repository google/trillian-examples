Layout
======

This doc describes the on-disk layout and format of the serverless log files.

> :warning: this is still under development, and the exact layout and contents
> of the serverless log structure may well change.

Broadly, the layout consists of a self-contained directory hierarchy whose
contents represents the entire state of the log.
The contents and structure of this directory is designed to allow it to be safely
and indefinitely cached by clients, with one exception - the checkpoint file.

Inside the directory you'll find:

 * :page_facing_up: checkpoint
 * :file_folder: seq/
 * :file_folder: leaves/
 * :file_folder: tile/

checkpoint
----------
`checkpoint` contains the latest log checkpoint in the format described
[here](/formats/log).
This is the *only* file in the serverless log data set which *should not* be
indefinitely cached by serving infrastructure or clients.

seq/
----
`seq/` contains a directory hierarchy containing leaf data for each sequenced
entry in the log.

To avoid creating a large directory containing many files, the following scheme
is used: the sequence number is interpreted as a 48 bit number, e.g.
`0x123456789a`, and a prefix directory hierarchy is created from that like so:
`.../seq/12/34/56/78/9a`.

leaves/
-------
`leaves/` contains files which map all known leaf hashes to their position in
the log.

Similarly to `seq/`, a prefix directory scheme is used: given a leaf hash
`0x0123456789ABCDEF0000000000000000`, the file used to store this leaf's index
in the log would be: `.../leaves/01/23/45/6789abcdef0000000000000000`.

The contents of the file is simply the hex ASCII string representation of the
index.

tile/
-----
`tile/` contains the internal nodes of the log tree.

The nodes are grouped into "tiles" - these are 8 level deep "sub trees".
Each tile exists at a particular location in tile-space, identified by its
`stratum` and `index`.

A *perfect* tree of size 2^16 would be represented by two strata of tiles:
 - stratum 0: 256 tiles wide, containing all the leaf hashes of the log along with
   the 7 bottom-most levels of internal nodes.
 - stratum 1: 1 tile wide, containing the next 8 levels of internal nodes.

Given an `index` of `0x0123456789` and an `stratum` of `0xef`, the corresponding
fully populated tile file would be found at `.../tile/ef/0123/45/67/89`.

Given that we would like tiles to be indefinitely cacheable, and that in
the general case a log will not be comprised entirely of perfect subtrees, tiles
which are *not* fully populated will be stored with a hex suffix representing
the number of (tile) "leaves" present in the tile.  E.g., for the `stratum` and
`index` given above, if the tile contained `0xab` (tile) "leaves" it would be
found at `.../tile/ef/0123/45/67/89.ab`

Tile file contents are a serialised [`Tile struct`](../../api/state.go) object.

Note that only finalised/non-ephemeral nodes are stored in tiles.
i.e. given the following tree, only nodes `[a]`, `[b]`, `[c]`, and `[x]` would
be stored in the tile file (but clients can re-compute `[y]` if they need to):
```
      [y]
     /   \
   [x]    \
  /   \    \
[a]   [b]   [c]
```