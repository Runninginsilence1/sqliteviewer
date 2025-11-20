#!/usr/bin/env just --justfile

build:


update:
  go get -u
  go mod tidy -v

lint:
  golangci-lint run

fmt:
  go fmt ./...