---
title: Authoring your tool inventory
description: Why partial OpenAPI registration is a game-changer for agent capabilities.
---

# Authoring your tool inventory

Giving an AI agent access to a massive corporate API — like an ERP, Stripe, or a monolithic internal service — is usually a mistake. 

If you hand an agent a 200-endpoint Swagger spec, you exhaust its context window with endpoints it will never need, you confuse the planner with ten different ways to search for a customer, and you drastically increase the blast radius if the model hallucinates.

Contenox lets you register any OpenAPI service without writing glue code. But more importantly, it lets you **author the subset**.

## The glue-code tax

Normally, restricting an agent to a safe subset of an API means writing an integration layer. You write a tool schema. You write a Python function. You map arguments, inject auth headers, make the HTTP call, and parse the JSON. If you want 5 tools, you write 5 wrapper functions.

In Contenox, you write zero code. You just slice the spec.

## Curating the inventory

Take an enormous legacy API. Pull down its `openapi.json` and delete the 198 endpoints the agent shouldn't touch. Leave the two it needs: `GET /invoices/{id}` and `POST /support/escalate`.

Save that as `support-subset.yaml` in your repository. Then register it:

```bash
contenox tools add erp_support \
  --url https://erp.internal.example.com \
  --spec ./specs/support-subset.yaml \
  --inject "tenant_id=acme"
```

The model never sees the other 198 endpoints. It never sees the `tenant_id` you injected. It just sees a logical tools named `erp_support` with two clear capabilities.

## Inventories, not monoliths

Because the spec path (`--spec`) is decoupled from where the traffic goes (`--url`), you can take *one* monolithic API and register it as *three different tools* in Contenox, each backed by a different subset spec.

```bash
contenox tools add erp_billing --spec ./specs/billing.yaml
contenox tools add erp_vacation --spec ./specs/hr.yaml
contenox tools add erp_inventory --spec ./specs/warehouse.yaml
```

Now, instead of handing a chain the entire `erp` monolith, you hand it exactly what it needs:

```json
{
  "execute_config": {
    "tools": ["erp_vacation", "local_shell"]
  }
}
```

## Why authoring matters

The subset spec is a file in your repo. You version it. You review it in PRs. If the agent needs a new capability, you add an endpoint to the YAML. 

You didn't write an integration function, but you authored the boundary. The agent only has the capabilities you explicitly curated into its inventory.
