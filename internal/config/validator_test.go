package config

import "testing"

func TestValidateAllowsAsyncWALReplication(t *testing.T) {
	cfg := Defaults()
	cfg.Replication.Role = "primary"
	cfg.Replication.ListenAddr = "127.0.0.1:7401"
	if err := Validate(cfg); err != nil {
		t.Fatalf("primary: %v", err)
	}
	cfg.Replication.Role = "replica"
	cfg.Replication.ListenAddr = ""
	cfg.Replication.PrimaryAddr = "127.0.0.1:7401"
	if err := Validate(cfg); err != nil {
		t.Fatalf("replica: %v", err)
	}
}

func TestValidateRejectsDataPlaneRaft(t *testing.T) {
	cfg := Defaults()
	cfg.Replication.RaftEnabled = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected data-plane Raft to fail closed")
	}
}

func TestValidateStandaloneRejectsStrayReplicaAddrs(t *testing.T) {
	cfg := Defaults()
	cfg.Replication.ListenAddr = "127.0.0.1:7401"
	if err := Validate(cfg); err == nil {
		t.Fatal("standalone config must not carry listen_addr")
	}
}

func TestTenantEngineStartsWithoutCheckoutYAML(t *testing.T) {
	cfg := TenantEngine(t.TempDir(), 6401, 8081)
	if err := ApplyReplication(cfg, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if cfg.Persistence.DataDir == "" || cfg.Replication.RaftEnabled {
		t.Fatalf("%+v", cfg.Persistence)
	}
	cfg.Replication.Role = "primary"
	cfg.Replication.ListenAddr = "127.0.0.1:7401"
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
}
