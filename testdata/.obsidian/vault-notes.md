# Vault Notes

This file lives inside a dot directory (`.obsidian/`). It demonstrates that
dot directories containing markdown now appear in the sidebar tree and are
reachable by wiki links — previously the tree walk skipped every directory
whose name started with a dot.

## What Changed

- The sidebar tree walk no longer prunes dot directories.
- The wikilink file index no longer skips them either, so `[[vault-notes]]`
  resolves from anywhere in the vault.
- Dot directories with **no** markdown files (like `.git`) still don't
  appear — empty branches are pruned from the tree as before.

## Try It

- Open the sidebar: `.obsidian/` is listed and expandable.
- From any other page, `[[vault-notes]]` links here.

> [!IMPORTANT]
> Dot directories are served exactly like regular ones — anything inside
> the served root is public to whoever can reach the server. Only serve
> directories you intend to share.
