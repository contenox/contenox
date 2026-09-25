"""The probes: unauthenticated, always on, and independent of what mounted.

`/healthz` says the process is serving and deliberately says nothing about the
database — a probe that fails on a database fault turns a degraded relay into a
restart loop. These tests hold that line: they must pass on a relay started
with nothing but a database path.
"""


def test_healthz_is_ok(client, base_url):
    r = client.get(f"{base_url}/healthz")
    assert r.status_code == 200, r.text
    assert r.json() == {"status": "ok"}


def test_version_carries_a_version(client, base_url):
    r = client.get(f"{base_url}/version")
    assert r.status_code == 200, r.text
    assert "version" in r.json()
    assert isinstance(r.json()["version"], str)


def test_the_probes_need_no_session(client, base_url):
    """Stated as its own test because it is the property that makes them usable
    as probes: a health check behind auth cannot be a health check."""
    client.cookies.clear()
    for path in ("/healthz", "/version"):
        assert client.get(f"{base_url}{path}").status_code == 200, path
