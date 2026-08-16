package etcd

import (
	"context"
	"testing"
	"time"

	"github.com/sjlit/aeus/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const testEndpoint = "localhost:2379"

func skipIfNoEtcd(t *testing.T) {
	t.Helper()
	cli, err := clientv3.New(clientv3.Config{
		Endpoints: []string{testEndpoint},
	})
	if err != nil {
		t.Skipf("no etcd client available: %v", err)
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := cli.Status(ctx, testEndpoint); err != nil {
		t.Skipf("no etcd server available at %s: %v", testEndpoint, err)
	}
}

func TestStatus_WithoutInit_ReturnsUnavailable(t *testing.T) {
	reg := New()
	hc, ok := reg.(registry.HealthChecker)
	if !ok {
		t.Fatal("etcd registry must implement registry.HealthChecker")
	}
	status, err := hc.Status(context.Background())
	if err == nil {
		t.Fatalf("expected error for uninitialized registry, got status=%v", status)
	}
	if status != registry.HealthUnavailable {
		t.Fatalf("expected HealthUnavailable, got %v", status)
	}
}

func TestStatus_Healthy(t *testing.T) {
	skipIfNoEtcd(t)

	reg := New()
	hc, ok := reg.(registry.HealthChecker)
	if !ok {
		t.Fatal("etcd registry must implement registry.HealthChecker")
	}
	if err := reg.Init(registry.WithAddress(testEndpoint)); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	status, err := hc.Status(ctx)
	if err != nil {
		t.Fatalf("expected healthy status, got error: %v", err)
	}
	if status != registry.HealthOK {
		t.Fatalf("expected HealthOK, got %v", status)
	}
}
