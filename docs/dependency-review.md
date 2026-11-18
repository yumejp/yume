# Dependency review

As of 2026-11-13. The Go toolchain minimum and Docker builder are 1.26.11:
govulncheck found reachable standard-library advisories in 1.26.2 (raised to
1.26.6 on 2026-09-08), in 1.26.6 (crypto/tls, crypto/x509, net/http,
net/textproto) and in 1.26.10 (net/http, GO-2026-6859). CI runs govulncheck
alongside the server tests.

The mobile lockfile still reports 14 npm audit findings: four high and ten
moderate, including dependent-package counts. These trace to two packages:

- `image-size@1.2.1` through React Native's CLI and Metro. Its image parsers can
  loop on crafted input ([ICNS](https://github.com/advisories/GHSA-w3rx-r6r6-pgpr),
  [JXL/HEIF](https://github.com/advisories/GHSA-5p2g-fcmc-qvqq)). Metro uses it to
  inspect build assets. Treat untrusted build assets as a risk until patched.
- `uuid@7.0.3` through Expo config plugins and `xcode`. The advisory concerns
  supplied buffers in v3/v5/v6; the inspected xcode caller uses `uuid.v4()`.
  [Advisory](https://github.com/advisories/GHSA-w5hq-g745-h8pq).

These paths are build tooling. The final Docker image contains the Go server
and exported web assets, not Node or node_modules, which narrows exposure but
does not resolve the audit findings. Do not force npm's proposed Expo
downgrade; revisit compatible upstream fixes before each release.
