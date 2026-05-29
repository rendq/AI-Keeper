# Contributing to AIP

Thank you for your interest in contributing to AIP! This guide covers how to get involved.

## Ways to Contribute

- Report bugs via [GitHub Issues](https://github.com/ai-keeper/ai-keeper/issues)
- Suggest features via [GitHub Discussions](https://github.com/ai-keeper/ai-keeper/discussions)
- Submit pull requests for fixes or new features
- Improve documentation
- Write tests (especially PBT properties)
- Create industry packs

## Development Setup

```bash
# Clone
git clone https://github.com/ai-keeper/ai-keeper.git
cd aip

# Install toolchain
make bootstrap

# Verify everything builds
go build ./...
go test ./... -count=1
```

### Requirements

- Go 1.22+
- Python 3.11+ with `uv`
- Node 20+ with `pnpm`
- Docker (for e2e tests)
- `pre-commit` (installed by `make bootstrap`)

## Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes
4. Run checks: `make lint && go test ./... -count=1`
5. Commit with DCO sign-off: `git commit -s -m "feat: add X"`
6. Push and open a Pull Request

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add Cedar policy conflict analyzer
fix: handle nil pointer in Skill dependency resolver
docs: add multi-region operations guide
test: add PBT for budget enforcement
chore: update controller-runtime to v0.19
```

## DCO Sign-Off

All commits must include a `Signed-off-by` line (Developer Certificate of Origin):

```
git commit -s -m "feat: your message"
```

This certifies you have the right to submit the contribution under the project's license.

## Pull Request Guidelines

- Keep PRs focused — one feature or fix per PR
- Include tests for new functionality
- Update docs if behavior changes
- Ensure CI passes (lint, unit, kubeconform)
- Reference related issues: `Fixes #123`

## Code Style

| Language | Formatter | Linter |
|----------|-----------|--------|
| Go | `gofumpt` | `go vet` + `staticcheck` |
| Python | `ruff format` | `ruff check` |
| YAML | — | `yamllint` |
| TypeScript | `prettier` | `eslint` |

Run `make lint` to check all at once.

## Testing

```bash
# Unit tests
go test ./... -count=1

# PBT (property-based tests)
go test ./... -tags=pbt -count=1

# Python tests
pytest dataplane/ -v

# E2E (requires kind cluster)
make e2e-up
go test ./test/e2e/... -tags=e2e -v
```

## Adding a New CRD

1. Create types in `api/<group>/v1alpha1/`
2. Add kubebuilder markers
3. Run `make manifests` to generate CRD YAML
4. Write controller in `controllers/<resource>/`
5. Add unit tests + at least one PBT property
6. Update Helm chart in `deploy/helm/ai-keeper/`

## Adding an Industry Pack

1. Create directory under `packs/industry/<name>/`
2. Include: `pack.yaml`, `manifests/*.yaml`, `values.yaml`, `README.md`
3. Follow existing packs (legal-copilot, finance, healthcare) as templates
4. Add eval sets under `eval/`

## Questions?

Open a [Discussion](https://github.com/ai-keeper/ai-keeper/discussions) — we're happy to help.
