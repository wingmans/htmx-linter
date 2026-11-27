.DEFAULT_GOAL := help

DIR ?= .
FLAGS ?=

.PHONY: help build run

help:
	@printf '%s\n' \
		'make help   Show this help' \
		'make build  Build bin/htmx-linter' \
		'make run    Build and scan DIR (default: .)' \
		'' \
		'Example: make run DIR=../chopstick/internal/chopstick/web FLAGS=-warnings-as-errors'

build:
	@mkdir -p bin
	go build -o bin/htmx-linter ./cmd/htmx-linter

run: build
	./bin/htmx-linter $(FLAGS) -- "$(DIR)"
