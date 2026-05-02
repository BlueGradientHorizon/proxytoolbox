.PHONY: build cli worker-singbox worker-xraycore clean

EXT :=
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

build: cli worker-singbox worker-xraycore

cli:
	cd internal/cli && make

worker-singbox:
	cd internal/workers/singbox && make

worker-xraycore:
	cd internal/workers/xraycore && make

run: build
	./bin/cli$(EXT) --worker-debug

clean:
ifeq ($(OS),Windows_NT)
	if exist bin rmdir /s /q bin
else
	rm -rf bin/
endif