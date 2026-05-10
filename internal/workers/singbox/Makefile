.PHONY: build

# Prefer GOOS set by the orchestrator; fallback to host OS for standalone use
_TARGET_OS := $(GOOS)
ifeq ($(_TARGET_OS),)
    ifeq ($(OS),Windows_NT)
        _TARGET_OS := windows
    else
        _TARGET_OS := linux
    endif
endif

ifeq ($(_TARGET_OS),windows)
    EXT := .exe
else ifeq ($(_TARGET_OS),android)
    EXT := .so
    PREFIX := lib
else
    EXT :=
    PREFIX :=
endif

WORKER_NAME := singbox-worker
GO_TAGS     := with_utls,with_quic

# OUTDIR is passed by the orchestrator as an absolute path
OUTDIR ?= ../../../bin
OUTPUT  = $(OUTDIR)/$(PREFIX)$(WORKER_NAME)$(EXT)

build:
	go build -o $(OUTPUT) $(if $(GO_TAGS),-tags $(GO_TAGS)) .