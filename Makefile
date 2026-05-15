VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT        ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE          ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
SKILL_VERSION ?= $(shell ./scripts/skill-version.sh 2>/dev/null || echo "dev")

LDFLAGS = -X github.com/airbytehq/airbyte-agent-cli/cmd.Version=$(VERSION) \
          -X github.com/airbytehq/airbyte-agent-cli/cmd.Commit=$(COMMIT) \
          -X github.com/airbytehq/airbyte-agent-cli/cmd.Date=$(DATE) \
          -X github.com/airbytehq/airbyte-agent-cli/cmd.ExpectedSkillVersion=$(SKILL_VERSION)

.PHONY: build generate test lint vet fmt check install clean

build: generate
	go build -ldflags "$(LDFLAGS)" -o airbyte-agent .

generate:
	go generate ./...

test:
	go test ./... -v

fmt:
	gofmt -w .
	go mod tidy

vet:
	go vet ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. Install: brew install golangci-lint"; exit 1; }
	golangci-lint run

check: fmt vet lint test

install:
	go install -ldflags "$(LDFLAGS)"

clean:
	rm -f airbyte-agent
