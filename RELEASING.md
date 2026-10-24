# Releasing Kero

A release is a tag: pushing `vX.Y.Z` runs the Release workflow
(`.github/workflows/release.yml`), which builds the app for macOS, Windows
and Linux, the server build for each platform, runs the tests, and leaves
it all in a draft release. Publishing the draft releases it.

Kero's releases are those of `smallnest/kero`, the repository `mygo.json`
(`updates.github`), `internal/update` (`Repo`) and the install scripts
name. Push the tag there.

## Signing

This fork has no Apple Developer ID, so macOS builds are ad-hoc signed
(`mygo.json`'s `macos.signingIdentity` is `-`) and not notarized. A user
who downloads the disk image must Control-click **Kero.app** and choose
**Open** the first time; `README.md` says so. The Release workflow has
no certificate or notarization steps.

The one secret a release needs is `MYGO_UPDATER_PRIVATE_KEY`, the secret
half of the `mygo keygen` pair whose public half is `mygo.json`'s
`updates.publicKey`. It signs the app's updates and the server build's
archives; without it, neither updates. Losing it strands every app that
already runs this fork's builds.

The key lives at `~/.kero-release-keys/mygo-update.key`, outside the
repository. Set it on the repository with:

```sh
gh secret set MYGO_UPDATER_PRIVATE_KEY --repo smallnest/kero < ~/.kero-release-keys/mygo-update.key
```

Apps installed from `egoist/kero` releases use that repository's key,
not this one, so they do not update to this fork's releases by
themselves: their users install one of these releases once.

## Steps

1. Pick the version: a patch (2.1.4 → 2.1.5) for fixes and small
   changes, a minor for features. The version of this fork is above
   upstream's latest, so no tag here shares a number with an upstream
   tag, which would make the update key of one repository sign the
   release of the other. Upstream's notes that a release takes in follow
   the fork's own in `CHANGELOG.md`.
2. Set it in `mygo.json` (`"version"`).
3. Add its section to the top of `CHANGELOG.md`: what changed since the
   last release (`git log vX.Y.Z..HEAD`), for the people who use Kero,
   not for who reads the code. Name settings as the app shows them
   (Settings › Hosts), credit outside contributors (Thanks @name), and
   say when hosts get a change with their next update. Leave out what
   changes nothing a user would notice, and fixes not confirmed to work.
4. Commit both as `Release X.Y.Z`, tag it `vX.Y.Z`, and push `dev` and
   the tag:

   ```sh
   git commit -am "Release X.Y.Z"
   git tag vX.Y.Z
   git push origin dev
   git push origin vX.Y.Z
   ```

5. Wait for the Release run of the tag to finish (about 5 minutes):

   ```sh
   gh run list --workflow release.yml --limit 1
   gh run watch <run id> --exit-status
   ```

6. Once every job succeeded, publish the draft, without asking:

   ```sh
   gh release edit vX.Y.Z --draft=false
   ```

   Where `gh` can't edit releases, run the Publish release workflow
   (`.github/workflows/publish-release.yml`) on `dev` instead: it
   publishes the draft of `mygo.json`'s version.

   If a job failed, leave the draft unpublished and report what failed.
   Fixed, the workflow can run again by hand (Run workflow on `dev`),
   which replaces the draft and moves the tag to that commit; a
   published release stays as it is.

The tag must match `mygo.json`'s version, or the run stops at its first
job. Any tag that starts with `v` starts a Release run, so do not tag
`dev` with a name like `v2.1.12-fork`: the run stops, and it is noise. Kero is released, so changes keep older apps, kerods and config
files working: `proto.Version` goes up only for a breaking change to the
protocol, not for a new optional field or frame.
