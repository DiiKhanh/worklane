.PHONY: test cover

test:
	go test ./...

cover:
	go test -coverprofile=cover.out ./services/otp-api/internal/... && go tool cover -func=cover.out | tail -1
