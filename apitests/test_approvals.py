"""HTTP-level coverage for the durable approval inbox (runtime/internal/approvalapi).

The inbox (slice C2) exposes pending human-in-the-loop asks over REST so an
operator can answer them without attaching to the session that raised them:
`GET /approvals` lists pending asks newest-first, `POST /approvals/{id}` answers
one. These tests pin the list shape and the answer-validation errors against a
live `contenox serve`.

A hermetic serve raises no gated tool call on its own, so the pending list is
empty here — which is exactly the "a fleet with nothing pending renders empty"
acceptance the inbox slice calls out, and lets these tests run without a real
approval in flight.
"""

from helpers import api_url, assert_status_code


def test_approvals_list_is_empty_array_shape(api, base_url):
    response = api.get(api_url(base_url, "approvals"), timeout=15)
    assert_status_code(response, 200)
    body = response.json()
    assert isinstance(body, list)
    # Nothing pending in a hermetic serve: the empty inbox is a JSON [], not an
    # error (hitlservice.ListPending's non-nil guarantee).
    assert body == []


def test_approvals_list_honours_limit_param(api, base_url):
    response = api.get(api_url(base_url, "approvals", limit="10"), timeout=15)
    assert_status_code(response, 200)
    assert isinstance(response.json(), list)


def test_approvals_answer_unknown_id_is_404(api, base_url):
    # Answering an ask that does not exist is 404 — there is nothing to answer.
    # An already-resolved or expired ask would be 409, but neither is reachable
    # without a real in-flight approval, so the unknown-id path is what a
    # hermetic suite can pin.
    response = api.post(
        api_url(base_url, "approvals", "no-such-approval-apitest"),
        json={"approved": True},
        timeout=15,
    )
    assert_status_code(response, 404)


def test_approvals_answer_deny_unknown_id_is_404(api, base_url):
    # The deny path resolves through the same handler; an unknown id is 404
    # regardless of the answer value.
    response = api.post(
        api_url(base_url, "approvals", "no-such-approval-apitest"),
        json={"approved": False},
        timeout=15,
    )
    assert_status_code(response, 404)
