"""Who may see an account's rows, asserted from the outside.

The property is INDISTINGUISHABILITY: an account that is not yours and an
account that does not exist must answer the same way, or the difference between
them is an oracle that tells a stranger which accounts are real.

The STATUS is recorded rather than assumed. The relay's HTTP layer answers 403
"you are not a member of that account" (internal/accountapi → requireActiveMember,
whose own comment claims "the same indistinguishable refusals"), while
internal/store/store.go states the opposite rule for its queries — "a
cross-tenant resource reads as not-found, not forbidden". Both agree on the
property this file tests; they disagree on the code. If the wire should be 404,
that is a change in the relay, and [NOT_YOURS] is where these tests would learn
about it.
"""
from __future__ import annotations

from conftest import account_id_of

# A well-formed id that belongs to nobody, so "not yours" and "does not exist"
# can be compared rather than assumed to agree.
NO_SUCH_ACCOUNT = "00000000-0000-4000-8000-000000000000"

# What the relay answers for an account the caller is not a member of. See the
# module docstring: this is the HTTP layer's choice, not the store's rule.
NOT_YOURS = 403


def test_you_can_read_your_own_account(alice, base_url):
    r = alice.get(f"{base_url}/v1/accounts/{account_id_of(alice, base_url)}")
    assert r.status_code == 200, r.text


def test_a_stranger_cannot_read_your_account(alice, bob, base_url):
    r = bob.get(f"{base_url}/v1/accounts/{account_id_of(alice, base_url)}")
    assert r.status_code == NOT_YOURS, r.text


def test_a_stranger_cannot_read_your_members(alice, bob, base_url):
    r = bob.get(f"{base_url}/v1/accounts/{account_id_of(alice, base_url)}/members")
    assert r.status_code == NOT_YOURS, r.text


def test_a_stranger_cannot_read_your_machines(alice, bob, base_url):
    r = bob.get(f"{base_url}/v1/accounts/{account_id_of(alice, base_url)}/instances")
    assert r.status_code == NOT_YOURS, r.text


def test_an_account_that_does_not_exist_answers_the_same_way(alice, bob, base_url):
    """The comparison, not the code: this is the whole test."""
    not_yours = bob.get(f"{base_url}/v1/accounts/{account_id_of(alice, base_url)}")
    nobody = bob.get(f"{base_url}/v1/accounts/{NO_SUCH_ACCOUNT}")
    assert nobody.status_code == not_yours.status_code, (
        f"a stranger's account answers {not_yours.status_code} and one that does not "
        f"exist answers {nobody.status_code}, which tells the difference"
    )


def test_the_refusal_is_not_an_authentication_failure(alice, bob, base_url):
    """Signed in and refused is not the same as signed out and refused: a 401
    here would send a signed-in client to the sign-in screen."""
    r = bob.get(f"{base_url}/v1/accounts/{account_id_of(alice, base_url)}/members")
    assert r.status_code != 401, r.text
