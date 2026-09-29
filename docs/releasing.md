# Publishing versions

Status: **plan** — nothing below is built yet. Once done, this page becomes
the release checklist.

## Goal

Every release produces, from one git tag:

| What | Where | How people use it |
| --- | --- | --- |
| GitHub Action | GitHub Marketplace | `uses: donaldwasserman/lgtm@v1` |
| Command-line binaries | GitHub Releases page | download and run |
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

Each release gets an exact tag (`v1.4.2`) that never moves, plus a moving
major tag (`v1`) that always points at the newest `v1.x.y`. Users pin `@v1` to
get fixes automatically, or `@v1.4.2` to stay put.

Docker images get the same tags without the `v`: `1.4.2`, `1.4`, `1`, `latest`.

## Decisions needed before v1.0.0

1. **Module name.** `go.mod` says `module lgtm`, so `go install` from GitHub
   can't work. Change to `module github.com/donaldwasserman/lgtm` and update
   imports. *Recommended.*
2. **License.** The repo has none, so nobody can legally use it. Pick one
   (MIT or Apache-2.0 are common) and add `LICENSE`.
3. **Marketplace name.** Must be unique across the Marketplace; `lgtm` is
   almost certainly taken. Suggest **"LGTM Review Gate"**. This is only the
   listing title — `uses: donaldwasserman/lgtm@v1` doesn't change.
4. **How the action gets the binary.** Today it compiles from source on every
   run, so users must install Go (~20–40s per run). Options:
   - **A. Download the release binary** (recommended). Fast, no Go needed.
     Falls back to compiling when run from a branch instead of a release.
   - B. Run the Docker image. No Go needed, but slower to start and
     Linux-only.
   - C. Keep compiling. Simplest, but every user needs `setup-go`.

## Work plan

### 1. Groundwork

- [ ] Rename the Go module (decision 1).
- [ ] Add `LICENSE` (decision 2).
- [ ] Add `lgtm --version`, set at build time from the git tag.
- [ ] Add a test workflow (`.github/workflows/ci.yml`) on every push and pull
      request: `make test`, `make test-action`, `make check`, `make scenarios`,
      `make check-action`, `make scenarios-action`. Today nothing runs the tests.
- [ ] Add `CHANGELOG.md`.
- [ ] Remove `.idea/` from git and add it to `.gitignore`.

### 2. Docker image

- [ ] Add a `Dockerfile` in two stages: build in `golang:1.25` with
      `CGO_ENABLED=1`, then copy only the binary into a small base image
      (`gcr.io/distroless/cc-debian12`; try a fully static build on
      `distroless/static` if it works).
- [ ] Entry point is `lgtm`, so `docker run IMAGE --base /a --head /b` works.
- [ ] Label the image with the repo URL so it shows up on the repo page.
- [ ] Build for `linux/amd64` and `linux/arm64`.
- [ ] Build (don't push) the image in the test workflow so a broken
      `Dockerfile` is caught before release.

### 3. Release workflow

New `.github/workflows/release.yml`, run when a `v*` tag is pushed:

1. Run all tests. Stop if any fail.
2. Build binaries. The parser is C code, so each platform builds on its own
   runner rather than cross-compiling:
   - `linux/amd64` on `ubuntu-latest`
   - `linux/arm64` on `ubuntu-24.04-arm`
   - `darwin/arm64` on `macos-latest`
   - `darwin/amd64` on an Intel macOS runner (e.g. `macos-15-intel`), or skip Intel Macs
   - (Windows later, if asked for.)
3. Package each as `lgtm_<version>_<os>_<arch>.tar.gz` plus a
   `checksums.txt`.
4. Build and push the Docker image to `ghcr.io` with the four tags above.
5. Create a **draft** GitHub Release with the files attached and notes from
   `CHANGELOG.md`.
6. Move the major tag (`v1`) to this commit.

Step 5 is a draft because Marketplace publishing can only be done by a person
in the GitHub web page (there is no API for it).

### 4. Action changes

- [ ] Add `author` and `branding` (icon and color) to `action.yml` — required
      for the Marketplace.
- [ ] Set `name` to the Marketplace name (decision 3).
- [ ] If decision 4 is A: replace the "Build lgtm" step with "download the
      binary for this release, check it against `checksums.txt`, else compile".
      The action needs to know its own exact version, since `@v1` doesn't say
      which `v1.x.y` it is; keep it in a `VERSION` file updated as part of each
      release.
- [ ] Add a test that the action works when used as `@<tag>` from another
      repository (a small example repo, or a job in `ci.yml` that runs the
      action from the just-pushed tag).

### 5. Marketplace (one time)

- [ ] Repository must be public, with `action.yml` at the top level and no
      other `action.yml` elsewhere.
- [ ] Accept the Marketplace developer agreement (needs two-factor auth on the
      account).
- [ ] Open the first draft release, tick **"Publish this Action to the GitHub
      Marketplace"**, choose categories, publish.

### 6. Docs

- [ ] Change every `donaldwasserman/lgtm@main` to `@v1`.
- [ ] Drop `actions/setup-go` from the Quick start (if decision 4 is A or B).
- [ ] Add install options to the README: download, `go install`, Docker.
- [ ] Add version and Marketplace badges to the README.

## Releasing a version (after the above is done)

1. Make sure `main` is green.
2. Update `CHANGELOG.md` and `VERSION`; merge.
3. Tag: `git tag v1.4.3 && git push origin v1.4.3`.
4. Wait for the release workflow to finish.
5. Open the draft release on GitHub, check the notes, tick **Publish to
   Marketplace**, publish.
6. Try it: `uses: donaldwasserman/lgtm@v1.4.3` in a test repository, and
   `docker run ghcr.io/donaldwasserman/lgtm:1.4.3 --version`.

If a release is broken: don't delete or move its exact tag. Fix forward with a
new patch release; the moving `v1` tag follows automatically.
