# jj on-disk formats read by this port

Formats match jj 0.42 (jj-lib sources referenced below). Everything here is
read-only; this tool never writes to a repo.

## Workspace layout (`workspace.rs`)

- `.jj/repo` — repo directory, or a file whose contents are a relative path
  to the primary workspace's repo directory (secondary workspaces).
- `.jj/working_copy/checkout` — protobuf `Checkout`
  (`local_working_copy.proto`): the workspace name (empty = `default`).

## Operations and views (`simple_op_store.rs`)

- `.jj/repo/op_heads/heads/<op-id-hex>` — empty files naming current
  operation heads. Multiple heads mean unmerged concurrent operations; jj
  merges them on next write, we pick the one with the latest end time.
- `.jj/repo/op_store/operations/<op-id-hex>` — protobuf `Operation`.
- `.jj/repo/op_store/views/<view-id-hex>` — protobuf `View`: heads,
  wc commit ids by workspace, local bookmarks/tags, remote views.

`RefTarget` terms are interleaved `[add0, remove0, add1, ..., addN]`; a
single non-absent term is a normal ref, a single absent term means deleted.

## Git backend (`git_backend.rs`)

- `.jj/repo/store/git_target` — path (possibly relative to the store dir)
  of the git dir holding commits/trees.
- Change id: `change-id` commit header in reverse hex (`z-k` digits, 16
  bytes), else the extras table, else synthesized from the commit id (last
  16 bytes, byte- and bit-reversed).
- Conflicted trees: `jj:trees` commit header with space-separated tree ids
  (odd count, alternating add/remove); a commit is conflicted iff its root
  tree has multiple terms.
- Extras table `.jj/repo/store/extra/` — stacked table keyed by commit id,
  value is protobuf `git_store.Commit` (change id, legacy conflicted root
  trees).
- The virtual root commit (all-zero id) is synthesized: no parents, empty
  description, empty git tree.

## Stacked tables (`stacked_table.rs`)

`heads/` holds empty files naming head segments. Segment file (all u32
little-endian):

    u32   parent file name length (0 = none)
    [...] parent file name (hex)
    u32   entry count N
    N ×   [key bytes][u32 value offset]   sorted by key
    [...] concatenated values

## Default index (`default_index/`)

- `.jj/repo/index/type` — must be `default`.
- `.jj/repo/index/op_links/<op-id-hex>` — protobuf `SegmentControl` naming
  the commit segment (jj ≥ 0.33). If the head operation has no link, jj
  builds one; we instead walk ancestor operations for the nearest link.
- `.jj/repo/index/segments/<hash>` — commit index segment, format v6.
  Layout after the `u32 version`, `u32 parent-name len`, parent name, and
  four `u32` counts (local commits, local change ids, parent overflow,
  change overflow):

      graph entries      16 + commit-id-len bytes each, parents-first order:
                         u32 generation, u32 parent1, u32 parent2,
                         u32 change-id lookup pos, commit id
      commit lookup      u32 local pos per entry, sorted by commit id
      change id table    change ids, sorted
      change pos table   u32 local pos (or overflow ref) per change id
      parent overflow    u32 global positions
      change overflow    u32 local positions

  Values ≥ `0x8000_0000` in parent/change-pos fields are bit-negated
  overflow references; `0xffff_ffff` ("no parent") negates to zero, making
  the zero-parent case fall out of the overflow path naturally.

Global position = segment's cumulative parent-commit count + local
position. Parents always have smaller global positions than children.

## Known deviations from jj-lib

- Divergent op heads are resolved by picking the newest op, not merging.
- Missing index links fall back to an ancestor operation's index instead of
  building; with no index at all, divergence detection is skipped and the
  change-id prefix length falls back to the configured display length.
- `empty` for merge commits is verified by entry-level tree merging only;
  merges needing file-content merging, criss-cross ancestry, or conflicted
  parents report non-empty (jj computes the full auto-merge).
- Commits missing from the extras table are not imported (jj-lib writes
  them); the change-id header/synthesis path covers them.
- SHA-1 repos only (`commitIDLength = 20`).
- Bookmarks at equal ancestor distance are sorted by name; the Rust version
  leaves them in unspecified (hash map) order.
