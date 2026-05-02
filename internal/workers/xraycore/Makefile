.PHONY: build

EXT :=
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

build:
	go build -o ../../../bin/xraycore-worker$(EXT) -trimpath .