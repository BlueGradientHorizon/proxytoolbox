.PHONY: build proxytoolbox tester-singbox clean

EXT :=
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

build: proxytoolbox tester-singbox

proxytoolbox:
	cd cmd/proxytoolbox && make

tester-singbox:
	cd cmd/testers/singbox && make

run: build
	./bin/proxytoolbox$(EXT)

clean:
ifeq ($(OS),Windows_NT)
	if exist bin rmdir /s /q bin
else
	rm -rf bin/
endif