.PHONY: build

EXT :=
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

build:
	go build -o ../../../bin/singbox-tester$(EXT) -tags with_utls,with_quic .