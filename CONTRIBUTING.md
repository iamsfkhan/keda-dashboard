# Contributing

Thank you for helping improve KEDA Dashboard.

1. Search existing issues before opening a proposal.
2. Keep changes focused and preserve the read-only security model.
3. Add or update tests and documentation with behavior changes.
4. Run `make test`, `make lint`, `make helm-lint`, and `make validate-rbac`.
5. Use clear commits and describe user impact, security implications, and verification in the pull request.

Go code should be formatted with `gofmt`. Frontend code must pass TypeScript, ESLint, and Vitest. Helm changes must retain non-root execution, probes, resource defaults, and least-privilege RBAC. New Kubernetes permissions require an explicit rationale.

By contributing, you agree that your contributions are licensed under Apache-2.0.

Please follow the [Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities according to [SECURITY.md](SECURITY.md), not in public issues.
