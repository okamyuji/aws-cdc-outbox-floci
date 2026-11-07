# Rails実装の品質ゲート。ルートのMakefileから委譲される
# テストはMySQL(127.0.0.1:3306, root/空パスワード)が起動している前提
.PHONY: gate precommit lint security test

APPS := source_api target_api
# mise activateはリポジトリ直下のRubyをPATHに置いたままにし、cdしても切り替えない。各アプリの.ruby-versionで動かすため、miseがあればmise execを通す
# miseは既定で.ruby-versionを読まない（idiomatic_version_file_enable_toolsの既定は空）ので、rubyだけ有効にする
RUN := $(if $(shell command -v mise 2>/dev/null),MISE_IDIOMATIC_VERSION_FILE_ENABLE_TOOLS=ruby mise exec --,)

lint:
	@for app in $(APPS); do (cd $$app && $(RUN) bin/rubocop) || exit 1; done

# git hookはGIT_DIRを渡し、worktreeでは絶対パスになる。bundler-auditが脆弱性DBで実行するgit pullがこのrepoを操作しないように外す
security:
	@for app in $(APPS); do (cd $$app && $(RUN) bin/brakeman --no-pager -q && env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE $(RUN) bin/bundler-audit check --update --quiet) || exit 1; done

test:
	@for app in $(APPS); do (cd $$app && APP_ENV=local $(RUN) bin/rails db:test:prepare test) || exit 1; done

gate: lint security test
	@echo "RAILS GATE PASS"

precommit: lint security
	@echo "RAILS PRECOMMIT PASS"
