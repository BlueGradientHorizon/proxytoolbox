.PHONY: build clean

build:
	go build -o bin/ -tags with_utls,with_quic

clean:
	rm -rf bin/