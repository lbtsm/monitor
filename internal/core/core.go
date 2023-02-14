package core

import (
	"fmt"
	"github.com/ChainSafe/log15"
	"github.com/mapprotocol/monitor/internal/chain"
	"os"
	"os/signal"
	"syscall"
)

type Core struct {
	Registry []chain.Chain
	log      log15.Logger
	sysErr   <-chan error
}

func New(sysErr <-chan error) *Core {
	return &Core{
		Registry: make([]chain.Chain, 0),
		log:      log15.New("system", "core"),
		sysErr:   sysErr,
	}
}

// AddChain registers the chain in the Registry and calls Chain.SetRouter()
func (c *Core) AddChain(chain chain.Chain) {
	c.Registry = append(c.Registry, chain)
}

// Start will call all registered chains' Start methods and block forever (or until signal is received)
func (c *Core) Start() {
	for _, chain := range c.Registry {
		err := chain.Start()
		if err != nil {
			c.log.Error(
				"failed to start chain",
				"chain", chain.Id(),
				"err", err,
			)
			return
		}
		c.log.Info(fmt.Sprintf("Started %s chain", chain.Name()))
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigc)

	// Block here and wait for a signal
	select {
	case err := <-c.sysErr:
		c.log.Error("FATAL ERROR. Shutting down.", "err", err)
	case <-sigc:
		c.log.Warn("Interrupt received, shutting down now.")
	}

	// Signal chains to shutdown
	for _, chain := range c.Registry {
		chain.Stop()
	}
}

func (c *Core) Errors() <-chan error {
	return c.sysErr
}
