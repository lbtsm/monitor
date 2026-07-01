package tron

import (
	"sync"

	"github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/monitor/internal/chain"
	"github.com/mapprotocol/monitor/internal/config"
	"github.com/mapprotocol/monitor/pkg/ethereum"
)

type Chain struct {
	cfg    *config.ChainConfig // The config of the chain
	conn   chain.Connection    // The chains connection
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
	conn := ethereum.NewConnection(cfg.ApiUrl, true, logger, cfg.GasLimit, cfg.MaxGasPrice, cfg.GasMultiplier)
	err = conn.Connect()
	if err != nil {
		return nil, err
	}

	tronConn := NewConn(cfg.Endpoint, logger)
	err = tronConn.Connect()
	if err != nil {
		return nil, err
	}

	// simplified a little bit
	var listen chain.Listener
	cs := chain.NewCommonSync(conn, cfg, logger, stop, sysErr)
	listen = NewMonitor(cs, tronConn)

	return &Chain{
		cfg:    chainCfg,
		conn:   conn,
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
		if c.conn != nil {
			c.conn.Close()
		}
	})
}

func (c *Chain) Id() config.ChainId {
	return c.cfg.Id

}

// UpdateCfg forwards a config mutation to the listener (used by hot reload).
func (c *Chain) UpdateCfg(fn func(*config.OptConfig)) {
	c.listen.UpdateCfg(fn)
}
