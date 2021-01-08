BINDIR       := $(CURDIR)/bin
INSTALL_PATH ?= /usr/local/bin
DIST_DIRS    := find * -type d -exec
TARGETS      := linux/amd64
TARGETS_OBJS ?= linux-amd64.tar.gz

# go option
PKG       := ./..
TAGS      :=
TESTS     := .
TESTFLAGS :=
LDFLAGS   := -w -s
GOFLAGS   :=

# Rebuild the buinary if any of thest files change
SRC := $(shell find . -type f -name '*.go' -print) go.mod go.sum

GIT_COMMIT = $(shell git rev-parse HEAD)
GIT_SHA    = $(shell git rev-parse --short HEAD)
GIT_TAG    = $(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
GIT_DIRTY  = $(shell test -n "`git status --porcelain`" && echo "dirty" || echo "clean")

ifdef VERSION
	BINARY_VERSION = $(VERSION)
endif
BINARY_VERSION ?= $(GIT_TAG)

.PHONY: all
all: build

# ---------------------------------------------------------------
# build

.PHONY: build
build: build_srs_hook build_rpc_server build_rpc_client


# build srs_hook
.PHONY: build_srs_hook
build_srs_hook: $(BINDIR)/srs_hook

$(BINDIR)/srs_hook: $(SRC)
	GO111MODULE=on go build $(GOFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/srsHook


# build rpc_server
.PHONY: build_rpc_server
build_rpc_server: $(BINDIR)/rpc_server

$(BINDIR)/rpc_server: $(SRC)
	GO111MODULE=on go build $(GOFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/rpc_server


# build rpc_client
.PHONY: build_rpc_client
build_rpc_client: $(BINDIR)/rpc_client

$(BINDIR)/rpc_client: $(SRC)
	GO111MODULE=on go build $(GOFLAGS) -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/rpc_client


# ---------------------------------------------------------------
# clean

.PHONY: clean
clean:
	@rm -f ${BINDIR}/*

# ---------------------------------------------------------------
# info

.PHONY: info
info:
	@echo "Version:        ${VERSION}"
	@echo "Git Tag:        ${GIT_TAG}"
	@echo "Git Commit:     ${GIT_COMMIT}"
	@echo "Git Tree State: ${GIT_DIRTY}"

