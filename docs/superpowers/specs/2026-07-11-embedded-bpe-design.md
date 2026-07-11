# Embedded BPE Design

## Goal

Ship a self-contained benchmark executable that uses an embedded `cl100k_base.tiktoken` file by default while retaining `--bpe-file` as an optional external override.

## Design

The benchmark package embeds `cl100k_base.tiktoken` with `//go:embed`. `InitTiktoken` accepts an optional external path: when supplied, it validates and uses that file; otherwise it writes the embedded bytes to a uniquely created temporary file, configures the existing offline-only BPE loader with that path, initializes `cl100k_base`, then removes the temporary file.

The external-file path remains supported for diagnostics or deliberate vocabulary replacement. The default startup path has no network access and no dependency on a sidecar BPE file. Failure to create or write the temporary file returns a clear initialization error.

## Packaging

The Makefile no longer copies `cl100k_base.tiktoken` into each build directory. Each target directory contains only its platform executable. README and project guidance describe the default embedded behavior and the override flag.

## Testing

Add a test that initializes the tokenizer with no external path and verifies ordinary encoding works. Preserve the external-path test helper to ensure the override remains functional. Build and run all unit tests plus the macOS Make target to confirm no BPE sidecar is produced.
