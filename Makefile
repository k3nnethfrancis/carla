.PHONY: install setup build test lint
install: setup build
	.venv/bin/python scripts/install.py
setup:
	uv sync --locked
build:
	cd tui && go build -o ../bin/carla .
test:
	uv run --locked pytest -q
	cd tui && go test ./...
lint:
	uvx ruff check src tests scripts
	uvx ruff format --check src tests scripts
	cd tui && go vet ./...
