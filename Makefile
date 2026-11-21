.PHONY: test cover test-link test-link-schema test-link-integration

test:
	go test ./...

test-link:
	go test -race ./pkg/idgen ./pkg/contracts/link ./services/link-svc/... ./services/link-dispatcher/...

test-link-integration:
	go test -race -tags=integration ./services/link-svc/... ./services/link-dispatcher/... -count=1

test-link-schema:
	go test -tags=integration ./db/link -count=1

cover:
	go test -coverprofile=cover.out ./services/otp-api/internal/... && go tool cover -func=cover.out | tail -1
