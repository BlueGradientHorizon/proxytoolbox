.PHONY: build cli worker-singbox clean

EXT :=
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

build: cli worker-singbox

cli:
	cd internal/cli && make

worker-singbox:
	cd internal/workers/singbox && make

run: build
	./bin/cli$(EXT) --worker-debug

clean:
ifeq ($(OS),Windows_NT)
	if exist bin rmdir /s /q bin
else
	rm -rf bin/
endif