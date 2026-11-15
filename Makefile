.PHONY: test cover test-link test-link-schema

test:
	go test ./...

test-link:
	go test -race ./pkg/idgen ./pkg/contracts/link ./services/link-svc/internal/adapters/inbound/http

test-link-schema:
	go test -tags=integration ./db/link -count=1

cover:
	go test -coverprofile=cover.out ./services/otp-api/internal/... && go tool cover -func=cover.out | tail -1
