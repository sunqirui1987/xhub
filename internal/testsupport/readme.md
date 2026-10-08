# Isolated database fixtures

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

DatabaseURL reads XHUB_TEST_DATABASE_URL or returns the local Compose default, not an empty string. Postgres creates a private schema, returns a URL with search_path, and registers cleanup.
Unreachable databases normally skip, but XHUB_REGRESSION_STRICT=1 fails. Once connected, fixture creation errors fail the test. IAM and Store must be opened against the returned schema URL. Forced process termination can leave schemas requiring targeted cleanup.
This package imports neither IAM nor gateway, avoiding cycles. methodcomment_test checks exported-method documentation conventions rather than business behavior. Never expose DSNs or credentials in reports.

## Source responsibilities and entry points

### postgres.go

- [`func DatabaseURL() string`](postgres.go) — DatabaseURL returns the DSN the suites should connect to.
- [`func Postgres(t *testing.T, prefix string) string`](postgres.go) — Postgres returns a DSN whose search_path points at a schema created for this one test, and registers the cleanup that drops it. A private schema is what makes these suites safe to run against a database a gateway is also using: the test writes real accounts, real keys and real usage rows, and without the schema it would read the developer's own rows and leave its own behind. The prefix names the caller in the schema, so a schema leaked by a killed test run can be traced back to the package that made it. The test skips when the database is unreachable. It fails only when the database answers and the fixture cannot be built, because that is a real error rather than a missing dependency.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [methodcomment_test.go](methodcomment_test.go) | `TestProductionMethodsDocumentParameters` |

```bash
go test ./internal/testsupport -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
