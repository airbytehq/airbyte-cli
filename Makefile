VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS = -X github.com/airbytehq/airbyte-cli/cmd.Version=$(VERSION) \
          -X github.com/airbytehq/airbyte-cli/cmd.Commit=$(COMMIT) \
          -X github.com/airbytehq/airbyte-cli/cmd.Date=$(DATE)

.PHONY: build test lint install clean

build:
	go build -ldflags "$(LDFLAGS)" -o airbyte .

test:
	go test ./... -v

lint:
	golangci-lint run

install:
	go install -ldflags "$(LDFLAGS)"

clean:
	rm -f airbyte
