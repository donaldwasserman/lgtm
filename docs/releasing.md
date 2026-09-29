# Releasing

One git tag publishes everything:

| What | Where | How people use it |
| --- | --- | --- |
| GitHub Action | GitHub Marketplace ("LGTM Review Gate") | `uses: donaldwasserman/lgtm@v1` |
| Command-line binaries | [Releases page](https://github.com/donaldwasserman/lgtm/releases) | download and run |
| Docker image | `ghcr.io/donaldwasserman/lgtm` | `docker run ghcr.io/donaldwasserman/lgtm:1` |
| Go install | Go module proxy | `go install github.com/donaldwasserman/lgtm/cmd/lgtm@latest` |

## Version numbers

Versions look like `v1.4.2` (major.minor.patch).

- **Patch** (`v1.4.2 → v1.4.3`): bug fixes, same results.
- **Minor** (`v1.4 → v1.5`): new options or languages; existing setups behave the same.
- **Major** (`v1 → v2`): anything that can break someone's setup or flip a result:
  - removing or renaming an input, output, or flag
  - changing a default (thresholds, `check-name`, `trust-mode`)
  - changing the JSON report or exit codes
  - changing how an existing language is measured

Each release has an exact tag (`v1.4.2`) that never moves, and a major tag
(`v1`) that always points at the newest `v1.x.y`. Docker images use the same
numbers without the `v`: `1.4.2`, `1.4`, `1`, `latest`.

## How to release

1. Make sure `main` is green.
2. In one pull request: set `VERSION` to the new number (e.g. `1.4.3`), and
   add a `## 1.4.3` section at the top of `CHANGELOG.md`. Merge it.
3. Tag the merge commit and push the tag:
   ```bash
   git checkout main && git pull
   git tag v1.4.3 && git push origin v1.4.3
   ```
4. Wait for two workflows to go green:
   - **release** checks the tag matches `VERSION`, runs the tests, builds
     binaries, and creates a **draft** release.
   - **publish** builds the Docker image and pushes it as `1.4.3`.
5. On GitHub, open the draft release. Check the notes, tick **"Publish this
   Action to the GitHub Marketplace"**, and click **Publish**.
6. Publishing starts the **promote** workflow, which points the `1.4`, `1`
   and `latest` image tags and the `v1` git tag at this release. Once it's
   green, the release is live.
7. Try it: `uses: donaldwasserman/lgtm@v1.4.3` in a test repository, and
   `docker run --rm ghcr.io/donaldwasserman/lgtm:1.4.3 --version`.

Why the manual step: GitHub only lets a person publish to the Marketplace, so
the workflow stops at a draft. Until you click Publish, only the exact
`1.4.3` image and `v1.4.3` tag exist; nobody on `@v1` or `:latest` gets the
new version.

**If a release is broken:** don't delete or move its tag. Release a fix as the
next patch version; `v1` follows automatically.

**If the release workflow fails:** fix the problem on `main`, delete the tag
(`git push --delete origin v1.4.3 && git tag -d v1.4.3`) and any draft it
made, then tag again. This is safe only while the release is still a draft.

## What happens behind the scenes

| Workflow | Runs when | Does |
| --- | --- | --- |
| `ci.yml` | every push to `main` and pull request | tests, model checks, image smoke test |
| `publish.yml` | push to `main` | pushes image tags `main` and `sha-<commit>` |
| `publish.yml` | a `v1.2.3` tag is pushed | pushes image tag `1.2.3` |
| `release.yml` | a `v1.2.3` tag is pushed | checks, builds Linux and macOS binaries (amd64 + arm64), creates a draft release with checksums |
| `promote.yml` | a release is published | points image tags `1.2`, `1`, `latest` and git tag `v1` at the release |

The action picks its image from the version it was used at: `@v1` or
`@v1.2.3` pulls that release's image (the number is read from `VERSION`), a
commit SHA pulls `sha-<commit>`, and anything else pulls `main`.

## One-time Marketplace setup

Done once, before the first release:

- [ ] Repository is public.
- [ ] Accept the Marketplace developer agreement (needs two-factor auth).
      GitHub prompts for this the first time you tick the Marketplace box.
- [ ] On the first release, pick the Marketplace categories (suggested:
      *Code review*, *Continuous integration*).
- [ ] Open the `lgtm` package settings on GitHub and set its visibility to
      **public**. New packages start private, and other repositories can't
      pull a private image — the action would fail for everyone.
- [ ] If tag protection rules are on, allow GitHub Actions to update `v*`
      tags, or `publish.yml` can't move `v1`.
