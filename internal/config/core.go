package config

import "github.com/ChainSafe/chainbridge-utils/msg"

type ChainConfig struct {
	Name             string            // Human-readable chains name
	Id               msg.ChainId       // ChainID
	Endpoint         string            // url for rpc endpoint
	Network          string            //
	From             string            // address of key to use
	KeystorePath     string            // Location of key files
	NearKeystorePath string            // Location of key files
	Insecure         bool              // Indicated whether the test keyring should be used
	BlockstorePath   string            // Location of blockstore
	FreshStart       bool              // If true, blockstore is ignored at start.
	LatestBlock      bool              // If true, overrides blockstore or latest block in config and starts from current block
	Opts             map[string]string // Per chains options
}
