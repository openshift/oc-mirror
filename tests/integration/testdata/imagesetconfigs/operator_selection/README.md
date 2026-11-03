# Operator selection configs

These configs use our test catalog:

- `isc-heads.yaml`: newest version in every channel of every package.
- `isc-package-heads.yaml`: newest foo version.
- `isc-full-packages.yaml`: all foo versions.
- `isc-default-channel.yaml`: only bar's alpha channel, made the default.
- `isc-package-range.yaml`: baz versions 1.0.0 through 1.0.1, excluding 1.1.0.
- `isc-package-channel-range.yaml`: bar versions 0.2.0 through 1.0.0
  from alpha, with alpha as the default channel.

Source, destination, and template tests reuse these selections with
test-specific paths and settings.
