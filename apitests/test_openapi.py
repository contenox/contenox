"""Every path the committed spec documents is mounted on the running relay.

openapi-gen reads the HANDLERS, so a registered route always reaches the spec.
The failure this catches is the other direction: a path the spec still promises
and the binary no longer serves — which no compile error, no Go test and no
generated-spec diff notices.

The spec is parsed with a regex rather than a YAML library: the file is
generated, its path keys sit at a fixed indent, and a test is the wrong place to
acquire a dependency. `test_the_spec_still_parses` fails loudly if that changes,
so a silent zero-match cannot read as a pass.
"""
from __future__ import annotations

import re
from pathlib import Path

SPEC = Path(__file__).resolve().parent.parent / "docs" / "openapi.yaml"

# Surfaces whose mount depends on configuration this harness deliberately does
# not provide, so a 404 from them says nothing about the spec:
#   /api/{chat,generate,show,tags,version}  the Ollama-compatible model proxy,
#                                           mounted only with licensing
#   /v1/billing/*                           needs Stripe
#   /v1/license/*                           needs the licence authority key
#
# The list names the proxy's five paths rather than all of /api/ on purpose: the
# runtime control surface is ALSO served under /api — /api/backends, /api/models,
# /api/state, /api/providers/* — and it is mounted whenever the relay is, which
# this harness always is. Skipping the prefix would stop checking it.
UNMOUNTED_WITHOUT_CONFIG = (
    "/api/chat",
    "/api/generate",
    "/api/show",
    "/api/tags",
    "/api/version",
    "/v1/billing/",
    "/v1/license/",
)


def _documented_paths() -> list[str]:
    text = SPEC.read_text()
    return sorted({m.group(1) for m in re.finditer(r"^\s{4}(/[A-Za-z0-9/{}_-]+):", text, re.M)})


def test_the_spec_still_parses():
    paths = _documented_paths()
    assert len(paths) > 20, (
        f"parsed only {len(paths)} paths from {SPEC}; the spec's shape moved and "
        "this test has stopped checking anything"
    )


def test_every_documented_path_is_mounted(client, base_url):
    missing = []
    for path in _documented_paths():
        if path.startswith(UNMOUNTED_WITHOUT_CONFIG):
            continue
        # DELETE, not GET. "Not 404" is the wrong probe: several handlers answer
        # 404 themselves — /v1/invites/preview does it for a token it does not
        # recognise — so a GET cannot tell an unrouted path from a refusal, and
        # this test would report a mounted route as missing. A method the route
        # does NOT declare is answered 405 by the mux when the path is mounted
        # and 404 when it is not, and nothing here is authenticated, so the
        # probe cannot act on anything either way.
        url = base_url + re.sub(r"\{[^}]+\}", "probe", path)
        r = client.delete(url, timeout=5, allow_redirects=False)
        if r.status_code == 404:
            missing.append(path)
    assert not missing, f"documented in the spec but not mounted: {missing}"
