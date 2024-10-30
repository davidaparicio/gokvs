# ---------------------------------------------------------------------------- #
#              Creative Commons 4.0 by-nc © 2023 The GoKVs Authors             #
#                                                                              #
#                +--------------------------------------------+                #
#                |                                            |                #
#                |           ______      __ ___    __         |                #
#                |         / ____/___  / //_/ |  / /____      |                #
#                |        / / __/ __ \/ ,<  | | / / ___/      |                #
#                |       / /_/ / /_/ / /| | | |/ (__  )       |                #
#                |       \____/\____/_/ |_| |___/____/        |                #
#                |                                            |                #
#                +--------------------------------------------+                #
#                                                                              #
# Go Key-Value Store, demo of O'Reilly Cloud Native Go book (Matthew A. Titmus)#
#                                                                              #
# ---------------------------------------------------------------------------- #
#                                                                              #
# Copyright © 2023 David Aparicio david.aparicio@free.fr                       #
#                                                                              #
# ---------------------------------------------------------------------------- #

all: compile check-format lint test

# Variables and Settings
version     ?=  $(shell git name-rev --tags --name-only $(shell git rev-parse HEAD))# 0.0.1
target      ?=  gokvs
org         ?=  davidaparicio
authorname  ?=  David Aparicio
authoremail ?=  david.aparicio@free.fr
license     ?=  MIT
year        ?=  2024
copyright   ?=  Copyright (c) $(year)

COMMIT      := $(shell git rev-parse HEAD)
DATE        := $(shell date)## +%Y-%m-%d)
IMAGE_NAME  := $(shell basename $(PWD))
PORTP       := 8080
PORTT       := 8080
# https://docs.docker.com/reference/cli/docker/container/run/#publish

PKG_LDFLAGS := github.com/davidaparicio/gokvs/internal
GORELEASER_FLAGS ?= --snapshot --clean
CGO_ENABLED := 0
export CGO_ENABLED

compile: mod ## Compile for the local architecture ⚙
	@echo "Compiling..."
	go build -ldflags "\
	-s -w \
	-X '${PKG_LDFLAGS}.Version=$(version)' \
	-X '${PKG_LDFLAGS}.BuildDate=$(DATE)' \
	-X '${PKG_LDFLAGS}.Revision=$(COMMIT)'" \
	-o bin/$(target) .

.PHONY: run
run: ## Run the server
	@echo "Running...\n"
	@go run -ldflags "\
	-s -w \
	-X '${PKG_LDFLAGS}.Version=$(version)' \
	-X '${PKG_LDFLAGS}.BuildDate=$(DATE)' \
	-X '${PKG_LDFLAGS}.Revision=$(COMMIT)'" \
	cmd/server/server.go

# -X 'main.Version=$(version)' \
# -X 'main.AuthorName=$(authorname)' \
# -X 'main.AuthorEmail=$(authoremail)' \
# -X 'main.Copyright=$(copyright)' \
# -X 'main.License=$(license)' \
# -X 'main.Name=$(target)' \

.PHONY: goreleaser #oldv: 1.18.2 
goreleaser: ## Run goreleaser directly at the pinned version 🛠
	go run github.com/goreleaser/v2@v2.3.2 $(GORELEASER_FLAGS)

.PHONY: dockerbuild
dockerbuild: ## Docker build 🛠
	docker build -t $(IMAGE_NAME) .

.PHONY: docker
docker: ## Docker run 🛠
	docker run -it --rm -p $(PORTP):$(PORTT) $(IMAGE_NAME)

.PHONY: dockerfull
dockerfull: dockerbuild docker ## Docker build and run 🛠

.PHONY: mod
mod: ## Go mod things
	go mod tidy
	go get -d ./...

.PHONY: install
install: compile test ## Install the program to /usr/bin 🎉
	@echo "Installing..."
	sudo cp bin/$(target) /usr/local/bin/$(target)

.PHONY: test
test: compile ## 🤓 Run go tests
	@echo "Testing..."
	go test -v ./...

.PHONY: clean
clean: ## Clean your artifacts 🧼
	@echo "Cleaning..."
	rm -rvf dist/*
	rm -rvf release/*
	rm -rvf pkg/api/

.PHONY: help
help:  ## Show help messages for make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(firstword $(MAKEFILE_LIST)) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: format
format: ## Format the code using gofmt
	@echo "Formatting..."
	@gofmt -s -w $(shell find . -name '*.go' -not -path "./vendor/*")

.PHONY: check-format
check-format: ## Used by CI to check if code is formatted
	@gofmt -l $(shell find . -name '*.go' -not -path "./vendor/*") | grep ".*" ; if [ $$? -eq 0 ]; then exit 1; fi

.PHONY: lint
lint: ## Runs the linter
	golangci-lint run

.PHONY: check-editorconfig
check-editorconfig: ## Use to check if the codebase follows editorconfig rules
	@docker run --rm --volume=$(shell PWD):/check mstruebing/editorconfig-checker

.PHONY: doc
doc: ## Launch the offline Go documentation 📚
	@echo "open http://127.0.0.1:6060 and run godoc server..."
	open "http://127.0.0.1:6060"
	godoc -http=:6060 -play

.PHONY: fuzz
fuzz: ## Run fuzzing tests 🌀
	@echo "Fuzzing..."
#	go test -v -fuzz "Fuzz" -fuzztime 15s

.PHONY: benchmark
benchmark: ## Run benchmark tests 🚄
	@echo "Benchmarking..."
	go test -v -run=^$ -bench . -benchmem -benchtime=10s ./

.PHONY: sec
sec: ## Go Security checks code for security issues 🔒
	gosec ./...
	govulncheck ./...