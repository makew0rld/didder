# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.2.0] - 2022-12-20
### Changed
- Updated dither library to v2.3.0
  - When comparing colors, each channel is weighted according to human luminance perception (#14)

### Fixed
- Updated dither library to v2.3.0
  - Corrected Burkes matrix (dither#10)
  - Palette order no longer affects output (dither#9)

## [1.1.0] - 2021-05-09
### Added
- Support for transparent images (#1)
- `--recolor` can handle RGB**A** colors (#1)


## [1.0.0] - 2021-05-01
Initial release.
