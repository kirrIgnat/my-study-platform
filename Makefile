APP := cosmic-portfolio
CMD := ./cmd/server

.PHONY: run build test vet fmt check

run:
	go run $(CMD)

build:
	go build -trimpath -o $(APP) $(CMD)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal

check: fmt test vet
