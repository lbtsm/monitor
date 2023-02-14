// Copyright 2021 Compass Systems
// SPDX-License-Identifier: LGPL-3.0-only

package chain

import (
	"github.com/ChainSafe/chainbridge-utils/crypto/secp256k1"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mapprotocol/monitor/pkg/ethclient"
	nearclient "github.com/mapprotocol/near-api-go/pkg/client"
	"github.com/mapprotocol/near-api-go/pkg/types/key"
	"math/big"
)

type Listener interface {
	Sync() error
}

type Connection interface {
	Connect() error
	Keypair() *secp256k1.Keypair
	Opts() *bind.TransactOpts
	CallOpts() *bind.CallOpts
	LockAndUpdateOpts() error
	UnlockOpts()
	Client() *ethclient.Client
	EnsureHasBytecode(address common.Address) error
	LatestBlock() (*big.Int, error)
	WaitForBlock(block *big.Int, delay *big.Int) error
	Close()
}

type Chain interface {
	Start() error // Start chains
	Id() msg.ChainId
	Name() string
	Stop()
}

type NearConnection interface {
	Connect() error
	Keypair() *key.KeyPair
	Opts() *bind.TransactOpts
	CallOpts() *bind.CallOpts
	LockAndUpdateOpts() error
	UnlockOpts()
	Client() *nearclient.Client
	EnsureHasBytecode(address string) error
	LatestBlock() (*big.Int, error)
	WaitForBlock(block *big.Int, delay *big.Int) error
	Close()
}
