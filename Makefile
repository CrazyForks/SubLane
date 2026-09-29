.PHONY: setup dev dev-api dev-web build generate generate-check test lint check clean brand release publish publish-check changelog changelog-check release-notes

GO ?= go
PNPM ?= pnpm
VERSION ?= 0.1.0-dev
WEB_PORT ?= 5173
GIT_CLIFF ?= git-cliff
SQLC = $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
AIR ?= $(GO) run github.com/air-verse/air@v1.67.4

setup:
	$(GO) mod download
	$(PNPM) --dir web install --frozen-lockfile

dev:
	node scripts/dev.mjs --port=$(WEB_PORT)

dev-api:
	CGO_ENABLED=0 $(AIR) -c .air.toml

dev-web:
	$(PNPM) --dir web dev

build:
	$(PNPM) --dir web build
	CGO_ENABLED=0 $(GO) build -tags production -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/sublane ./cmd/sublane

generate:
	$(SQLC) generate

generate-check:
	$(SQLC) diff

test:
	node --test scripts/*.test.mjs
	$(GO) test -race ./...
	$(PNPM) --dir web test

lint:
	@for script in scripts/*.sh; do bash -n "$$script" || exit 1; done
	node --check scripts/dev.mjs
	node --check scripts/release.mjs
	node --check scripts/publish.mjs
	node --check scripts/changelog.mjs
	$(GO) vet ./...
	@test -z "$$(gofmt -l cmd internal web/*.go)" || (echo 'Run gofmt before checking.'; exit 1)
	$(PNPM) --dir web typecheck
	$(PNPM) --dir web lint
	$(PNPM) --dir web format:check

check: generate-check lint test build

clean:
	rm -rf bin web/dist dist

brand:
	$(PNPM) --dir tools/brand install --frozen-lockfile
	$(PNPM) --dir tools/brand build
	$(PNPM) --dir tools/brand check

# Frontend is built once; each archive includes its matching cross-compiled binary.
release:
	@node scripts/release.mjs metadata "v$(VERSION)" owner/repo >/dev/null
	$(PNPM) --dir web build
	@for arch in amd64 arm64; do \
		mkdir -p "dist/release/linux-$$arch"; \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch $(GO) build -tags production -trimpath -buildvcs=false -ldflags="-s -w -X main.version=$(VERSION)" -o "dist/release/linux-$$arch/sublane" ./cmd/sublane || exit 1; \
		bash scripts/package.sh "$(VERSION)" "$$arch" "dist/release/linux-$$arch/sublane" dist/release || exit 1; \
	done
	@cd dist/release && shasum -a 256 sublane_$(VERSION)_linux_*.tar.gz > SHA256SUMS

# Command-line variables are auto-exported by Make; keep TAG literal in the dedicated value.
unexport TAG
publish publish-check: export SUBLANE_PUBLISH_TAG = $(value TAG)

publish:
	node scripts/publish.mjs

publish-check:
	node scripts/publish.mjs --dry-run

export GIT_CLIFF
release-notes: export SUBLANE_CHANGELOG_TAG = $(value TAG)

changelog:
	node scripts/changelog.mjs

release-notes:
	node scripts/changelog.mjs --release

changelog-check:
	"$(GIT_CLIFF)" --version
	SUBLANE_TEST_CHANGELOG=1 node --test scripts/changelog.test.mjs
