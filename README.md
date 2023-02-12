# zig-toolchain

This is a small utility I wrote in Go to download and quickly switch versions of the [zig](http://ziglang.org) compiler.

Currently creates a symbolic link to the zig binary located at `~/.local/bin/zig`.

## Installation

```
git clone https://github.com/dmbfm/zig-toolchain.git
cd zig-toolchain
go install
```

## Usage

To download and activate the current master version:
```
zig-toolchain activate master
```

To download and activate a given version of the zig compiler, e.c., `0.9.1`:
```
zig-toolchain activate 0.9.1
```

To list the locally downloaded versions:
```
zig-toolchain show
```

To list the versions that are available for download:
```
zig-toolchain list
```
