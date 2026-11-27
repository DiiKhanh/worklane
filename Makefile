.PHONY: test cover test-link test-link-schema test-link-integration test-link-e2e

test:
	go test ./...

test-link:
	go test -race ./pkg/idgen ./pkg/contracts/link ./services/link-svc/... ./services/link-dispatcher/...

# Packages in one go test invocation share a Testcontainers reaper.
test-link-integration:
	go test -p 1 -race -tags=integration ./services/link-svc/... ./services/link-dispatcher/... -count=1

test-link-schema:
	go test -tags=integration ./db/link -count=1

test-link-e2e:
	go test -tags=e2e,linke2e ./test/e2e -run '^TestLinkE2E' -count=1 -v

cover:
	go test -coverprofile=cover.out ./services/otp-api/internal/... && go tool cover -func=cover.out | tail -1
