# relay apitests

Black-box HTTP tests against a **running relay**. They assert what the wire
does — status codes, what a body carries, who may see which row — and nothing
about SQL. Data shape, store invariants and tenancy at the query layer belong in
the Go tests beside the store (`internal/store`), so a failure here is always
about the HTTP surface.

The split is bob2's (`bob2/apitests` — deleted, see `docs/BREAK-GLASS.md`), and
the reason is the same: the two suites answer different questions, and a suite
that answers both tells you neither.

## Running

```bash
task relay:apitests
```

That creates `apitests/.venv`, installs `requirements.txt` and runs the suite.
The harness **starts its own relay**: a throwaway SQLite file, a generated
keypair, a free port, and `/healthz` polled until it answers.

Against a relay it did not start:

```bash
RELAY_BASE_URL=https://relay.contenox.com apitests/.venv/bin/pytest
```

The database reset is then skipped, so every test has to make its own state.

## What the fixtures give you

| fixture | what it is |
|---|---|
| `base_url` | where the relay is |
| `client` | a fresh `requests.Session` per test — no cookie survives into the next |
| `alice` / `bob` | signed-in owners of two *different* accounts |
| `clean_db` | autouse: empties every relay table before each test |

`clean_db` reads the table list **from the database** rather than from a list
written here, drops the event page tables, and leaves `relay_meta` and
`relay_event_nid_seq` alone. A table list in a test is stale the moment a table
is added, and silently so.

Helpers in `conftest.py`: `signup(...)` and `account_id_of(...)`.

## Conventions

- **Always assert with the body**: `assert r.status_code == 201, r.text`. A
  failing status is nearly always the relay explaining itself, and without it a
  failure prints only a number.
- **One file per HTTP surface**, not one per resource: `test_tenancy.py` is the
  cross-account rule everywhere it applies, because that rule is one rule.
- **A skipped check says why.** Anything excluded because this harness does not
  mount it is listed with its reason, not silently omitted.
