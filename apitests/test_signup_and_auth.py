"""Registration, sign-in and the session cookie.

The session is a cookie the relay sets and the client carries, which is why
these tests use `requests.Session` and why `client` is fresh per test: "is this
person signed in" is only a real question when nothing leaked in from before.
"""
from __future__ import annotations

from conftest import signup, signup_body

PASSWORD = "correcthorsebattery"


def _signup(client, base_url, email, password=PASSWORD, **extra):
    return client.post(f"{base_url}/v1/auth/signup", json=signup_body(email, password, **extra))


def test_signup_creates_an_account_and_signs_in(client, base_url):
    r = _signup(client, base_url, "alice@acme.test", account_name="Acme")
    assert r.status_code == 201, r.text

    session = r.json()["session"]
    assert session["user"]["email"] == "alice@acme.test"
    assert len(session["accounts"]) == 1, r.text
    assert session["accounts"][0]["name"] == "Acme"

    # The cookie from signup is a session: the same client can read itself.
    # `/me` returns the session UNWRAPPED, unlike the `session` object signup
    # puts in its own body.
    me = client.get(f"{base_url}/v1/auth/me")
    assert me.status_code == 200, me.text
    assert me.json()["user"]["email"] == "alice@acme.test"


def test_signup_creates_an_account_even_with_no_name(client, base_url):
    """An account is created either way: everything billable and everything
    paired hangs off one, and asking a solo operator to name a tenant before
    they have seen the product is asking a question they cannot answer."""
    r = _signup(client, base_url, "solo@acme.test")
    assert r.status_code == 201, r.text
    assert len(r.json()["session"]["accounts"]) == 1


def test_a_taken_address_is_refused(client, base_url):
    assert _signup(client, base_url, "dup@acme.test").status_code == 201
    again = _signup(client, base_url, "dup@acme.test")
    assert again.status_code == 409, again.text


def test_a_short_password_is_refused(client, base_url):
    r = _signup(client, base_url, "short@acme.test", password="tooshort")
    assert r.status_code == 400, r.text


def test_signin_with_the_right_password(client, base_url):
    signup(base_url, "alice@acme.test", PASSWORD, "Acme")
    r = client.post(
        f"{base_url}/v1/auth/login",
        json={"email": "alice@acme.test", "password": PASSWORD},
    )
    assert r.status_code == 200, r.text
    # Sign-in returns the session UNWRAPPED, like /me — unlike signup, which
    # puts the same object under "session".
    assert r.json()["user"]["email"] == "alice@acme.test"


def test_signin_with_the_wrong_password_is_401(client, base_url):
    signup(base_url, "alice@acme.test", PASSWORD, "Acme")
    r = client.post(
        f"{base_url}/v1/auth/login",
        json={"email": "alice@acme.test", "password": "not-the-password"},
    )
    assert r.status_code == 401, r.text


def test_an_address_that_does_not_exist_is_the_same_refusal(client, base_url):
    """One answer for a wrong password and for an address nobody holds:
    distinguishing them turns sign-in into an oracle for which addresses are
    registered."""
    wrong_password = client.post(
        f"{base_url}/v1/auth/login",
        json={"email": "alice@acme.test", "password": "not-the-password"},
    )
    unknown = client.post(
        f"{base_url}/v1/auth/login",
        json={"email": "nobody@nowhere.test", "password": PASSWORD},
    )
    assert unknown.status_code == wrong_password.status_code == 401, unknown.text


def test_me_without_a_session_is_401(client, base_url):
    r = client.get(f"{base_url}/v1/auth/me")
    assert r.status_code == 401, r.text


def test_logout_ends_the_session(client, base_url):
    registered = _signup(client, base_url, "alice@acme.test", account_name="Acme")
    assert registered.status_code == 201, registered.text
    signed_in = client.get(f"{base_url}/v1/auth/me")
    assert signed_in.status_code == 200, signed_in.text

    out = client.post(f"{base_url}/v1/auth/logout")
    assert out.status_code < 300, out.text
    after = client.get(f"{base_url}/v1/auth/me")
    assert after.status_code == 401, after.text
