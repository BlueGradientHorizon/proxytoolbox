.PHONY: build cli tester-singbox clean

EXT :=
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

build: cli tester-singbox

cli:
	cd internal/cli && make

tester-singbox:
	cd internal/testers/singbox && make

run: build
	./bin/cli$(EXT)

clean:
ifeq ($(OS),Windows_NT)
	if exist bin rmdir /s /q bin
else
	rm -rf bin/
endif