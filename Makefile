.PHONY: release sync-web sync-sums
release:
	@./scripts/release.sh

# Regenerate the admin web template from admin/web.
sync-web:
	@go generate ./cmd/aeus

# After a release is pushed: sync go.sum for released submodules.
sync-sums:
	@./scripts/sync-module-sums.sh $(MODULES)
