package security

import "testing"

func TestTenantQuotaRejectsOnlyTheNoisyTenant(t *testing.T) {
	mgr := NewTenantQuotaManager()
	mgr.SetQuota(&TenantQuota{TenantID: "noisy", MaxMemBytes: 100, MaxKeys: 10})
	mgr.SetQuota(&TenantQuota{TenantID: "quiet", MaxMemBytes: 1 << 20, MaxKeys: 1000})
	if err := mgr.CheckMemory("noisy", 50); err != nil {
		t.Fatal(err)
	}
	mgr.TrackWrite("noisy", 80, true)
	if err := mgr.CheckMemory("noisy", 50); err == nil {
		t.Fatal("expected noisy quota rejection")
	}
	if err := mgr.CheckMemory("quiet", 50); err != nil {
		t.Fatal(err)
	}
	mgr.TrackWrite("quiet", 50, true)
	usage := mgr.Usage("quiet")
	if usage.MemBytes != 50 || usage.KeyCount != 1 {
		t.Fatalf("%#v", usage)
	}
	if mgr.Usage("missing").MemBytes != 0 {
		t.Fatal("missing tenant")
	}
	if err := mgr.CheckMemory("unknown", 9999); err != nil {
		t.Fatal(err)
	}
}
