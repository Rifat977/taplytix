BIN      := bin/taplytix
PKG      := ./...
MAIN     := ./cmd/taplytix

.PHONY: build test run lint tidy clean

build:
	@mkdir -p bin
	go build -o $(BIN) $(MAIN)

test:
	go test $(PKG)

run: build
	$(BIN) start

lint:
	go vet $(PKG)
	gofmt -l . | tee /dev/stderr | (! read)

tidy:
	go mod tidy

clean:
	rm -rf bin
