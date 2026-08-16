package metadata

import (
	"context"
	"testing"
)

func TestMergeContext_PreservesExisting(t *testing.T) {
	ctx := Set(context.Background(), "a", "1")

	patch := New()
	patch.Set("b", "2")

	merged := MergeContext(ctx, patch, true)

	if v, ok := Get(merged, "a"); !ok || v != "1" {
		t.Fatalf("expected existing a=1 preserved, got %q (ok=%v)", v, ok)
	}
	if v, ok := Get(merged, "b"); !ok || v != "2" {
		t.Fatalf("expected patch b=2 added, got %q (ok=%v)", v, ok)
	}
	// merging must not pollute the original context
	if _, ok := Get(ctx, "b"); ok {
		t.Fatal("MergeContext must not mutate the original context")
	}
}

func TestMergeContext_OverwriteSemantics(t *testing.T) {
	ctx := Set(context.Background(), "a", "1")

	patch := New()
	patch.Set("a", "2")

	merged := MergeContext(ctx, patch, false)
	if v, _ := Get(merged, "a"); v != "1" {
		t.Fatalf("overwrite=false should keep a=1, got %q", v)
	}
	merged = MergeContext(ctx, patch, true)
	if v, _ := Get(merged, "a"); v != "2" {
		t.Fatalf("overwrite=true should set a=2, got %q", v)
	}
}

func TestMergeContext_DeleteOnEmptyValue(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		ctx := Set(context.Background(), "a", "1")

		patch := New()
		patch.Set("a", "")

		merged := MergeContext(ctx, patch, overwrite)
		if _, ok := Get(merged, "a"); ok {
			t.Fatalf("empty patch value should delete the key regardless of overwrite=%v", overwrite)
		}
	}
}

func TestMergeContext_NilPatch(t *testing.T) {
	ctx := Set(context.Background(), "a", "1")

	merged := MergeContext(ctx, nil, true)
	if v, ok := Get(merged, "a"); !ok || v != "1" {
		t.Fatalf("nil patch should preserve existing metadata, got %q (ok=%v)", v, ok)
	}
}

func TestMergeContext_NilCtx(t *testing.T) {
	merged := MergeContext(nil, nil, true)
	if merged == nil {
		t.Fatal("MergeContext(nil, nil) must not return nil")
	}
	Get(merged, "anything") // must not panic
}
