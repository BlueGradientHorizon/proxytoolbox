.PHONY: build proto cli worker-singbox worker-xraycore clean

EXT :=
ifeq ($(OS),Windows_NT)
EXT := .exe
endif

PROTO_SRC = worker/protocol.proto
PROTO_GEN = worker/protocol.pb.go

build: proto cli worker-singbox worker-xraycore

proto: $(PROTO_GEN)

$(PROTO_GEN): $(PROTO_SRC)
	protoc --go_out=. --go_opt=paths=source_relative $(PROTO_SRC)

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
	if exist worker\protocol.pb.go del worker\protocol.pb.go
else
	rm -rf bin/
	rm -f worker/protocol.pb.go
endif
