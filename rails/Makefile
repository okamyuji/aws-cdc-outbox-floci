# Rails実装の品質ゲート。ルートのMakefileから委譲される
# テストはMySQL(127.0.0.1:3306, root/空パスワード)が起動している前提
.PHONY: gate precommit lint security test

APPS := source_api target_api

lint:
	@for app in $(APPS); do (cd $$app && bin/rubocop) || exit 1; done

security:
	@for app in $(APPS); do (cd $$app && bin/brakeman --no-pager -q && bin/bundler-audit check --update --quiet) || exit 1; done

test:
	@for app in $(APPS); do (cd $$app && APP_ENV=local bin/rails db:test:prepare test) || exit 1; done

gate: lint security test
	@echo "RAILS GATE PASS"

precommit: lint security
	@echo "RAILS PRECOMMIT PASS"
