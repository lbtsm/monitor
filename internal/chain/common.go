package chain

import (
	"sync"
	"time"

	"github.com/ChainSafe/log15"
	"github.com/mapprotocol/monitor/internal/config"
)

// Common is the shared scaffolding embedded by every per-chain monitor.
// Cfg is a pointer so a hot-reload writer can mutate the OptConfig in place
// and have the polling goroutine observe the new values on its next loop.
//
// Writes that need atomicity (e.g. swapping the Users slice) MUST go through
// UpdateCfg so that readers using Snapshot see a consistent view.
type Common struct {
	Cfg    *config.OptConfig
	Conn   Connection
	Log    log15.Logger
	Stop   <-chan int
	MsgCh  chan struct{}
	SysErr chan<- error // Reports fatal error to core

	// Wg tracks the lifetime of background goroutines spawned by this
	// chain's listener. Sync() must Wg.Add(1) before launching its loop
	// and the loop must defer Wg.Done(); chain.Stop() then Wg.Wait()s on
	// the close(stop) signal so the connection isn't torn down while the
	// goroutine is still touching it.
	Wg sync.WaitGroup

	cfgMu sync.RWMutex
}

// NewCommonSync creates and returns a listener.
//
// The OptConfig is stored by pointer; the caller may keep a reference to it
// and mutate fields in place after construction (under cfgMu / UpdateCfg).
func NewCommonSync(conn Connection, cfg *config.OptConfig, log log15.Logger, stop <-chan int, sysErr chan<- error) *Common {
	return &Common{
		Cfg:    cfg,
		Conn:   conn,
		Log:    log,
		Stop:   stop,
		SysErr: sysErr,
		MsgCh:  make(chan struct{}),
	}
}

// Snapshot returns an independent value copy of the live OptConfig.
// Polling loops should call this once at the top of each iteration to obtain
// a consistent view for that iteration without holding the lock during work.
//
// Note: slice and pointer fields inside the snapshot still alias the live
// data; callers that intend to mutate them MUST not — treat the snapshot as
// strictly read-only.
func (c *Common) Snapshot() config.OptConfig {
	c.cfgMu.RLock()
	defer c.cfgMu.RUnlock()
	return *c.Cfg
}

// UpdateCfg invokes fn while holding the cfgMu write lock. This is the only
// supported way to mutate the live OptConfig fields when polling goroutines
// may be running.
func (c *Common) UpdateCfg(fn func(*config.OptConfig)) {
	c.cfgMu.Lock()
	fn(c.Cfg)
	c.cfgMu.Unlock()
}

// Wait blocks until all background goroutines tracked via Wg have exited.
// chain.Chain.Stop() typically calls this after close(stop) so the
// connection can be torn down without racing the polling loop.
func (c *Common) Wait() {
	c.Wg.Wait()
}

// SleepWithStop waits for d unless stop is closed first. It returns false
// when the caller should stop work immediately.
func SleepWithStop(stop <-chan int, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-stop:
		return false
	case <-timer.C:
		return true
	}
}
