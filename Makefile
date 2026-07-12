# 品質ゲート一式。CIとローカルで同一のターゲットを使う（単一真実源）
.PHONY: all fmt fmt-check vet lint staticcheck test cover vuln gate precommit tf-fmt tf-validate build-lambda e2e

GOFILES := $(shell find . -name '*.go' -not -path './terraform/*')

all: gate

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

gate: fmt-check vet lint staticcheck test cover vuln tf-validate
	@echo "GATE PASS"

# pre-commit用の軽量ゲート（testcontainersを伴うテストはCI/手動のgateで実行する）
precommit: fmt-check vet lint staticcheck vuln
	@echo "PRECOMMIT PASS"

tf-fmt:
	terraform fmt -recursive terraform/

tf-validate:
	terraform fmt -check -recursive terraform/
	cd terraform/envs/local && terraform init -backend=false -input=false > /dev/null && terraform validate
	cd terraform/envs/stg && terraform init -backend=false -input=false > /dev/null && terraform validate

# Lambda 2種をprovided.al2023 (arm64) 向けにビルドしてzip化する
build-lambda:
	mkdir -p dist
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o dist/fanout/bootstrap ./lambda/fanout
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o dist/delivery/bootstrap ./lambda/delivery
	cd dist/fanout && zip -q -j ../fanout.zip bootstrap
	cd dist/delivery && zip -q -j ../delivery.zip bootstrap

e2e:
	cd e2e && pnpm install --silent && pnpm exec playwright test
