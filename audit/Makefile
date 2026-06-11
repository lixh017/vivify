# Makefile for the audit harness.
# `make` (default) = full audit; `make pre-commit` = quick smoke; `make all` = same as default.
#
# Run from the audit dir (where this Makefile lives).

ROOT := $(shell pwd)
HOME_DIR := $(HOME)
SHELL := /bin/bash

.PHONY: all pre-commit score evals gates tests tokens quality behaviour clean help

all: pre-commit
	@echo "Full audit done — see above."

pre-commit: score evals gates tests tokens
	@echo ""
	@echo "✓ pre-commit suite green"

score:
	@node $(ROOT)/harness-audit.js repo --root $(HOME_DIR)/.claude

evals:
	@node $(ROOT)/run-evals.js --all

gates:
	@bash $(ROOT)/test-gate.sh

tests:
	@node $(ROOT)/tests/runner.js

tokens:
	@node $(ROOT)/validate-settings.js

quality:
	@node $(ROOT)/skill-quality.js --root $(HOME_DIR)/.claude/skills --top 10

behaviour:
	@node $(ROOT)/skill-test.js 2>&1 | tail -3

clean:
	@find $(ROOT) -name '*.bak' -delete
	@rm -rf $(ROOT)/coverage/test-results.txt

help:
	@echo "Targets:"
	@echo "  make (all)        - run pre-commit + report"
	@echo "  make pre-commit   - run score + evals + gates + tests + tokens"
	@echo "  make score        - just harness-audit scorecard"
	@echo "  make evals        - just the 20-eval baseline"
	@echo "  make gates        - 7-test gate smoke"
	@echo "  make tests        - 12 unit tests"
	@echo "  make tokens       - validate-settings"
	@echo "  make quality      - top-10 form scores"
	@echo "  make behaviour    - skill-test (claim tests)"
	@echo "  make clean        - remove backup files"
	@echo "  make help         - this help"
