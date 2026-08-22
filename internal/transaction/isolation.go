package transaction

// IsolationLevel defines transaction isolation level.
type IsolationLevel int

const (
	ReadCommitted   IsolationLevel = iota // See only committed writes
	RepeatableRead                        // Snapshot at transaction start
	Serializable                          // Full serialization
)

// IsolationEnforcer validates reads against a snapshot version.
type IsolationEnforcer struct {
	mvcc *MVCCStore
}

// NewIsolationEnforcer creates an enforcer.
func NewIsolationEnforcer(mvcc *MVCCStore) *IsolationEnforcer {
	return &IsolationEnforcer{mvcc: mvcc}
}

// ReadAtSnapshot returns the value at the given snapshot version.
func (i *IsolationEnforcer) ReadAtSnapshot(key string, snapshotVersion Version) *VersionedValue {
	return i.mvcc.ReadAt(key, snapshotVersion)
}

// Snapshot captures the current global version for repeatable reads.
func (i *IsolationEnforcer) Snapshot() Version {
	return i.mvcc.CurrentVersion()
}
