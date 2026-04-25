.PHONY: build proxytoolbox tester-singbox clean

EXT :=
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

build: proxytoolbox tester-singbox

proxytoolbox:
	go build -o bin/proxytoolbox$(EXT)

tester-singbox:
	go build -o bin/singbox-tester$(EXT) -tags with_utls,with_quic ./cmd/testers/singbox

clean:
ifeq ($(OS),Windows_NT)
	if exist bin rmdir /s /q bin
else
	rm -rf bin/
endif