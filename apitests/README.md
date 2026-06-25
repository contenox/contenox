# Contenox API Smoke Tests

These tests are recovered from the old OSS API test shape, but trimmed for the
current local `contenox serve` contract.

Run the suite with:

```sh
make test-api
```

The runner builds `bin/contenox`, creates a temporary HOME/workspace, runs
`contenox init --force`, starts `contenox serve` on `127.0.0.1:32124`, then runs
pytest against `http://127.0.0.1:32124/api`.

Useful overrides:

```sh
CONTENOX_APITEST_PORT=32125 make test-api
make test-api PYTEST_ARGS="-k taskchain"
APITEST_RUN_DOWNLOAD=1 make test-api PYTEST_ARGS="-k download_curated"
```

By default the suite avoids real providers, real model downloads, and the user's
real `~/.contenox` state.
