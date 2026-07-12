# Go実装の品質ゲート。ルートのMakefileから委譲される
.PHONY: fmt fmt-check vet lint staticcheck test cover vuln gate precommit build-lambda

GOFILES := $(shell find . -name '*.go')

fmt:
	gofmt -w $(GOFILES)

fmt-check:
	@out=$$(gofmt -l $(GOFILES)); if [ -n "$$out" ]; then echo "gofmt未適用: $$out"; exit 1; fi

vet:
	go vet ./...

lint:
	golangci-lint run ./...

staticcheck:
	staticcheck ./...

test:
	go test ./... -coverprofile=coverage.out -covermode=atomic

# main関数（配線のみ）を除いたカバレッジを報告する
cover: test
	@grep -v 'main.go' coverage.out > coverage_nomain.out
	@go tool cover -func=coverage_nomain.out | tail -1

vuln:
	govulncheck ./...

gate: fmt-check vet lint staticcheck test cover vuln
	@echo "GOLANG GATE PASS"

precommit: fmt-check vet lint staticcheck vuln
	@echo "GOLANG PRECOMMIT PASS"

# Lambda 2種をprovided.al2023 (arm64) 向けにビルドし、ルートのdist/へ配置する
build-lambda:
	mkdir -p ../dist/fanout ../dist/delivery
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o ../dist/fanout/bootstrap ./lambda/fanout
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o ../dist/delivery/bootstrap ./lambda/delivery
	cd ../dist/fanout && zip -q -j ../fanout.zip bootstrap
	cd ../dist/delivery && zip -q -j ../delivery.zip bootstrap
