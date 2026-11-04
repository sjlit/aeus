.PHONY: release release-staged release-publish sync-web sync-sums
release:
	@./scripts/release.sh

# Two-phase release: prepare the commit and root tag locally.
release-staged:
	@STAGED=1 ./scripts/release.sh

# Push the staged release and tag submodules at their go.sum-synced commits.
release-publish:
	@./scripts/release-publish.sh

# Regenerate the admin web template from admin/web.
sync-web:
	@go generate ./cmd/aeus

# Legacy path for an already-pushed release: sync go.sum for submodules.
sync-sums:
	@./scripts/sync-module-sums.sh $(MODULES)
