#!/usr/bin/env just --justfile

build:
  goreleaser build --snapshot --clean

release:
  goreleaser release --clean

update:
  go get -u
  go mod tidy -v

lint:
  golangci-lint run

fmt:
  go fmt ./...