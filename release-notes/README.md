# Release notes

One Markdown file per release, named after the tag: `release-notes/v0.1.2.md` for tag `v0.1.2`.

## How it works

The [release workflow](../.github/workflows/release.yml) runs on every `v*` tag. Before
GoReleaser builds the release it looks for `release-notes/<tag>.md`:

- **Found** — its contents become the GitHub release body (passed to GoReleaser as
  `--release-notes`).
- **Missing** — the release is still created; the body is just left to GoReleaser's default
  (empty, since the changelog is disabled in `.goreleaser.yml`).

So the notes file must be committed **before** the tag is pushed. The usual flow is:

1. Copy `TEMPLATE.md` to `release-notes/v<next version>.md` and fill it in.
2. Commit it along with the code for that release.
3. Tag and push: `git tag v<next version> && git push origin v<next version>`.

## Writing good notes

- Lead with user-facing changes: **Fixed**, **Added**, **Changed**, grouped under those headings.
- Name the resource/attribute/provider setting involved (for example `adds_gpo_links`,
  `gpo_powershell_path`), so readers can map a line to their config.
- Call out anything that changes behaviour on upgrade, and how to opt out.
- Keep internal-only changes (tests, refactors) in a short **Internal** section or omit them.

