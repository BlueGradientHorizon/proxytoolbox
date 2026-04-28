# Description

This is a Go library project called `proxytoolbox`. It's main purpose is to mass-scan VPN configs to filter out non-working. But it also has a feature to perform download and upload tests.

# Project structure

The `internal/cli` is a developer utility used for testing the library's functionality during development. It is not part of the library's public API.
The `internal/workers/singbox` is an example implementation of worker program. It is also not a part of library.

The actual library code is organized in the following packages:

TODO: describe directories

When working on this project, focus on the library code first and edit `internal/cli` last.

# Code conventions

Package `worker` should only be imported either by code from package `runner` or from worker implementation. `runner` is supposed to be public API for usual library usage which manages `worker`. 

# Building

ALWAYS use `make` to build the helper `internal/cli` program. NEVER make up build command "by hand" (`go build ...`, `go run ...`, etc). Use `go run ...` ONLY in situations when you need to build short code snippet to check out some specific info; after running delete binary and source file.
