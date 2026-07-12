# ルートの品質ゲート。golang/railsの両ゲートと共通インフラ検証を束ねる（単一真実源）
.PHONY: all gate precommit golang-gate rails-gate tf-fmt tf-validate build-lambda e2e

all: gate

golang-gate:
	$(MAKE) -C golang gate

rails-gate:
	@if [ -d rails ]; then $(MAKE) -C rails gate; else echo "rails未作成のためスキップ"; fi

gate: golang-gate rails-gate tf-validate
	@echo "GATE PASS"

precommit:
	$(MAKE) -C golang precommit
	@if [ -d rails ]; then $(MAKE) -C rails precommit; fi
	@echo "PRECOMMIT PASS"

tf-fmt:
	terraform fmt -recursive terraform/

tf-validate:
	terraform fmt -check -recursive terraform/
	cd terraform/envs/local && terraform init -backend=false -input=false > /dev/null && terraform validate
	cd terraform/envs/stg && terraform init -backend=false -input=false > /dev/null && terraform validate

build-lambda:
	$(MAKE) -C golang build-lambda

e2e:
	cd e2e && pnpm install --silent && pnpm exec playwright test
