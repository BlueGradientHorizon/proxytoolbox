# Description

This is a Go library project called `proxytoolbox`. It's main purpose is to mass-scan VPN configs to filter out non-working. But it also have a feature to perform download and upload tests.

# Project structure

The `main.go` file is a developer utility used for testing the library's functionality during development. It is not part of the library's public API.

The actual library code is organized in the following packages:
- `parsers/` - Protocol parsers (Hysteria2, Shadowsocks, Trojan, VLESS, VMess)
- `testers/` - Testing utilities (latency, speed)
- `printers/` - Output formatting
- `tools/` - Utility tools
- `utils/` - Helper functions

When working on this project, focus on the package code first and edit `main.go` last.

# Building

ALWAYS use `make build` to build the runnable executable. NEVER make up build command "by hand" (`go build ...`, `go run ...`, etc). Use `go run ...` ONLY in situations when you need to build short code snippet to check out some specific info; after running delete binary and source file.
