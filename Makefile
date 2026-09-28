.PHONY: install setup build test lint
install: setup build
	.venv/bin/python scripts/install.py
setup:
	uv sync --locked
build:
	uv run --locked python scripts/build.py
test:
	uv run --locked pytest -q
	cd tui && go test ./...
lint:
	uv run --locked ruff check src tests scripts
	uv run --locked ruff format --check src tests scripts
	cd tui && go vet ./...
