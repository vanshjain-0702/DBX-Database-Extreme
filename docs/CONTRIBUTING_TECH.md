# Technical Contribution Guide

Welcome to the DBX core engine! This document outlines how to build, test, and contribute to the technical infrastructure of DBX.

## Prerequisites
- Go 1.25+
- Node.js 20+ (for Dashboard)
- Python 3.10+ (for SDK and scripts)

## Building the Project
We use `make` to orchestrate builds.
```bash
# Build the DBX Server and Orchestrator binaries
make build

# Build the Docker images locally
make docker-build
```

## Running Tests
All pull requests must pass the CI pipeline, which includes data races and strict linting.
```bash
# Run Go tests (`make test` is the portable target; Linux CI adds -race)
make test

# Operator drills
make soak
make restore-drill

# Python SDK + examples (flake8, black, pytest)
make python-check

# Run the linter locally
golangci-lint run
```

Public site: edit [`website/`](../website/). Dashboard: edit [`dashboard/`](../dashboard/),
then `npm run lint` and `npm run build` (CI golangci-lint builds the dashboard first
so the Go embed is current).

## Dashboard Development
To work on the React Dashboard:
```bash
cd dashboard
npm install
npm run dev
```

Ensure you run `npm run lint` before committing any frontend changes.
