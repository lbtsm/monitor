package xrp

import (
	"sync"

	"github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/monitor/internal/chain"
	"github.com/mapprotocol/monitor/internal/config"
)

type Chain struct {
	cfg    *config.ChainConfig // The config of the chain
	stop   chan<- int
	listen chain.Listener
	once   sync.Once
}

func New(chainCfg *config.ChainConfig, logger log15.Logger, sysErr chan<- error, tks *config.Token,
	genni *config.Api, users []config.From) (*Chain, error) {
	cfg, err := config.ParseOptConfig(chainCfg, tks, genni, users)
	if err != nil {
		return nil, err
	}

	stop := make(chan int)
	netConn := NewConn(cfg.Endpoint, logger)
	err = netConn.Connect()
	if err != nil {
		return nil, err
	}

	// simplified a little bit
	var listen chain.Listener
	cs := chain.NewCommonSync(nil, cfg, logger, stop, sysErr)
	listen = NewMonitor(cs, netConn)

	return &Chain{
		cfg:    chainCfg,
		stop:   stop,
		listen: listen,
	}, nil
}

func (c *Chain) Name() string {
	return c.cfg.Name

}

func (c *Chain) Start() error {
	err := c.listen.Sync()
	if err != nil {
		return err
	}

	log.Debug("Successfully started chain")
	return nil
}

func (c *Chain) Stop() {
	c.once.Do(func() {
		close(c.stop)
		c.listen.Wait()
	})
}

func (c *Chain) Id() config.ChainId {
	return c.cfg.Id

}

func (c *Chain) UpdateCfg(fn func(*config.OptConfig)) {
	c.listen.UpdateCfg(fn)
}
