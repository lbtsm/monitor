package main

import (
	"github.com/ChainSafe/chainbridge-utils/msg"
	log "github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mapprotocol/monitor/chains/eth"
	"github.com/mapprotocol/monitor/chains/near"
	"github.com/mapprotocol/monitor/internal/chain"
	"github.com/mapprotocol/monitor/internal/config"
	"github.com/mapprotocol/monitor/internal/core"
	"github.com/mapprotocol/monitor/internal/mapprotocol"
	"github.com/urfave/cli/v2"
	"strconv"
)

var monitorCommand = cli.Command{
	Name:        "monitor",
	Usage:       "monitor account balance",
	Description: "The messenger command is used to sync the log information of transactions in the block",
	Action:      run,
	Flags:       append(app.Flags, config.FileFlag),
}

func run(ctx *cli.Context) error {
	err := startLogger(ctx)
	if err != nil {
		return err
	}

	log.Info("Starting Compass...")

	cfg, err := config.GetConfig(ctx)
	if err != nil {
		return err
	}

	// Check for test key flag
	var ks string
	ks = cfg.KeystorePath

	// Used to signal core shutdown due to fatal error
	sysErr := make(chan error)

	//mapId, err := strconv.Atoi(cfg.MapChain.Id)
	//if err != nil {
	//	return err
	//}
	c := core.New(sysErr)
	// merge map chains
	allChains := make([]config.RawChainConfig, 0, len(cfg.Chains)+1)
	allChains = append(allChains, cfg.MapChain)
	allChains = append(allChains, cfg.Chains...)

	for idx, ac := range allChains {
		chainId, err := strconv.Atoi(ac.Id)
		if err != nil {
			return err
		}
		// write Map chains id to opts
		ac.Opts[config.MapChainID] = cfg.MapChain.Id
		chainConfig := &config.ChainConfig{
			Name:             ac.Name,
			Id:               msg.ChainId(chainId),
			Endpoint:         ac.Endpoint,
			From:             ac.From,
			Network:          ac.Network,
			KeystorePath:     ks,
			NearKeystorePath: ac.KeystorePath,
			Opts:             ac.Opts,
		}
		var (
			newChain chain.Chain
		)

		logger := log.Root().New("chains", chainConfig.Name)
		if ac.Type == config.Near {
			newChain, err = near.InitializeChain(chainConfig, logger, sysErr)
			if err != nil {
				return err
			}
		} else {
			newChain, err = eth.InitializeChain(chainConfig, logger, sysErr)
			if err != nil {
				return err
			}
		}
		if idx == 0 {
			// assign global map conn
			mapprotocol.GlobalMapConn = newChain.(*eth.Chain).EthClient()
			mapprotocol.InitOtherChain2MapHeight(common.HexToAddress(chainConfig.Opts[config.LightNode]))
		}
		c.AddChain(newChain)
	}

	c.Start()

	return nil
}
