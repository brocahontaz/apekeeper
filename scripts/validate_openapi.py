#!/usr/bin/env python3
"""Dependency-free validation for the checked-in API contract."""
import json
from pathlib import Path

spec = json.loads(Path("openapi/openapi.json").read_text())
manifest = json.loads(Path("openapi/routes.json").read_text())
assert spec["openapi"].startswith("3."), "OpenAPI 3 is required"
assert spec["paths"], "the contract must describe at least one path"
assert "Error" in spec["components"]["schemas"]
actual = {f"{method.upper()} {path}": operation for path, item in spec["paths"].items() for method, operation in item.items() if not method.startswith("x-")}
assert set(actual) == set(manifest), f"OpenAPI routes drift: missing={sorted(set(manifest)-set(actual))}, extra={sorted(set(actual)-set(manifest))}"
for path, item in spec["paths"].items():
    assert path.startswith("/api/") or path in {"/healthz", "/readyz"}, f"unexpected path: {path}"
    for method, operation in item.items():
        if method.startswith("x-"):
            continue
        assert operation.get("responses"), f"{method.upper()} {path} has no responses"
        route = manifest[f"{method.upper()} {path}"]
        roles = operation.get("x-required-roles", [])
        expected = route["roles"]
        if expected == ["authenticated"]:
            # The session security scheme is the canonical declaration for
            # routes that need authentication but no guild role.
            assert operation.get("security") and roles in ([], ["any authenticated user"]), f"{method.upper()} {path} has inaccurate role declaration"
        elif expected:
            assert roles == expected, f"{method.upper()} {path} has inaccurate role declaration"
        else:
            assert not roles, f"{method.upper()} {path} must be public"
        for segment in path.split("/"):
            if segment.startswith("{"):
                names = {p["name"] for p in operation.get("parameters", []) if p.get("in") == "path"}
                assert segment[1:-1] in names, f"{method.upper()} {path} is missing its path parameter"
        if path.startswith("/api/") and path not in {"/api/auth/login", "/api/auth/callback", "/api/healthz", "/api/readyz"}:
            assert operation.get("security"), f"{method.upper()} {path} has no authentication declaration"
        if method.lower() in {"post", "put", "delete", "patch"} and operation.get("security"):
            assert any("session" in s and "csrf" in s for s in operation["security"]), f"{method.upper()} {path} must require session and CSRF together"
        for status, response in operation["responses"].items():
            if (status.startswith("4") or status.startswith("5")) and path not in {"/healthz", "/api/healthz", "/readyz", "/api/readyz"}:
                assert response.get("$ref") == "#/components/responses/Error", f"{method.upper()} {path} error response is not the stable envelope"
            if status.startswith("2") and status not in {"204", "302"} and path not in {"/healthz", "/api/healthz", "/readyz", "/api/readyz"}:
                assert response.get("content"), f"{method.upper()} {path} success response has no schema"
print(f"validated {len(spec['paths'])} API paths")
