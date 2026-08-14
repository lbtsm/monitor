package tron

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"google.golang.org/grpc"

	"github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/lbtsm/gotron-sdk/pkg/client"
	"github.com/lbtsm/gotron-sdk/pkg/common"
	"github.com/lbtsm/gotron-sdk/pkg/proto/api"
	"github.com/lbtsm/gotron-sdk/pkg/proto/core"
	"github.com/pkg/errors"
)

type Connection struct {
	endpoint                  string
	cli                       *client.GrpcClient
	log                       log15.Logger
	stop                      chan int
	reqTime, cacheBlockNumber int64
}

func NewConn(endpoint string, log log15.Logger) *Connection {
	return &Connection{
		endpoint: endpoint,
		log:      log,
		stop:     make(chan int),
	}
}

// Connect starts the ethereum WS connection
func (c *Connection) Connect() error {
	c.log.Info("Connecting to tron chain...", "url", c.endpoint)
	c.cli = client.NewGrpcClient(c.endpoint)
	err := c.cli.Start(grpc.WithInsecure())
	if err != nil {
		return err
	}
	return nil
}

func (c *Connection) Keypair() *keystore.Key {
	return nil
}

func (c *Connection) Client() *ethclient.Client {
	return nil
}

func (c *Connection) Opts() *bind.TransactOpts {
	return nil
}

func (c *Connection) CallOpts() *bind.CallOpts {
	return nil
}

func (c *Connection) UnlockOpts() {
}

func (c *Connection) LockAndUpdateOpts(needNewNonce bool) error {
	return nil
}

// LatestBlock returns the latest block from the current chain
func (c *Connection) LatestBlock() (*big.Int, error) {
	// 1s req
	if time.Now().Unix()-c.reqTime < 3 {
		return big.NewInt(0).SetInt64(c.cacheBlockNumber), nil
	}

	bnum, err := c.cli.GetNowBlock()
	if err != nil {
		return nil, err
	}
	c.cacheBlockNumber = bnum.GetBlockHeader().GetRawData().Number
	c.reqTime = time.Now().Unix()

	return big.NewInt(0).SetInt64(bnum.GetBlockHeader().GetRawData().Number), nil
}

// EnsureHasBytecode asserts if contract code exists at the specified address
func (c *Connection) EnsureHasBytecode(addr ethcommon.Address) error {
	return nil
}

func (c *Connection) WaitForBlock(targetBlock *big.Int, delay *big.Int) error {
	return nil
}

func (c *Connection) Close() {
	if c.cli != nil {
		_ = c.cli.Conn.Close()
	}
	close(c.stop)
}

const (
	delegationRequestGap = 500 * time.Millisecond
	queryMaxRetries      = 3
	queryRetryBackoff    = time.Second
	queryTimeout         = 15 * time.Second
)

// withRetry runs do up to attempts times with exponential backoff.
func withRetry(attempts int, backoff time.Duration, do func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = do(); err == nil {
			return nil
		}
		if i < attempts-1 {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return err
}

// InboundDelegations lists every energy delegation TO target (Stake 2.0).
// Any final failure returns an error — callers must treat the whole scan as
// UNKNOWN rather than compute from partial data.
//
// Note: the SDK's GetDelegatedResourcesV2 walks ToAccounts (outbound), which
// is the opposite direction, so we drive the raw stubs ourselves.
func (c *Connection) InboundDelegations(target string) ([]DelegationDetail, error) {
	targetBytes, err := common.DecodeCheck(target)
	if err != nil {
		return nil, errors.Wrapf(err, "decode address %s", target)
	}

	index, err := c.delegationIndex(targetBytes)
	if err != nil {
		return nil, errors.Wrap(err, "GetDelegatedResourceAccountIndexV2")
	}

	details := make([]DelegationDetail, 0, len(index.GetFromAccounts()))
	for i, from := range index.GetFromAccounts() {
		if i > 0 {
			time.Sleep(delegationRequestGap)
		}
		list, err := c.delegationDetail(from, targetBytes)
		if err != nil {
			return nil, errors.Wrapf(err, "GetDelegatedResourceV2 from %s", common.EncodeCheck(from))
		}
		for _, d := range list.GetDelegatedResource() {
			if d.GetFrozenBalanceForEnergy() <= 0 {
				continue // bandwidth-only delegation
			}
			details = append(details, DelegationDetail{
				From:             common.EncodeCheck(d.GetFrom()),
				FrozenBalanceSun: d.GetFrozenBalanceForEnergy(),
				ExpireTimeMs:     d.GetExpireTimeForEnergy(),
			})
		}
	}
	return details, nil
}

func (c *Connection) delegationIndex(target []byte) (*core.DelegatedResourceAccountIndex, error) {
	var index *core.DelegatedResourceAccountIndex
	err := withRetry(queryMaxRetries, queryRetryBackoff, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		defer cancel()
		var e error
		index, e = c.cli.Client.GetDelegatedResourceAccountIndexV2(ctx, client.GetMessageBytes(target))
		return e
	})
	return index, err
}

func (c *Connection) delegationDetail(from, to []byte) (*api.DelegatedResourceList, error) {
	var list *api.DelegatedResourceList
	err := withRetry(queryMaxRetries, queryRetryBackoff, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		defer cancel()
		var e error
		list, e = c.cli.Client.GetDelegatedResourceV2(ctx, &api.DelegatedResourceMessage{
			FromAddress: from,
			ToAddress:   to,
		})
		return e
	})
	return list, err
}

// EnergyResourceParams reads the numbers needed for sun→energy conversion.
func (c *Connection) EnergyResourceParams(target string) (ResourceParams, error) {
	var params ResourceParams
	err := withRetry(queryMaxRetries, queryRetryBackoff, func() error {
		res, e := c.cli.GetAccountResource(target)
		if e != nil {
			return e
		}
		params = ResourceParams{
			EnergyLimit:       res.GetEnergyLimit(),
			EnergyUsed:        res.GetEnergyUsed(),
			TotalEnergyLimit:  res.GetTotalEnergyLimit(),
			TotalEnergyWeight: res.GetTotalEnergyWeight(),
		}
		return nil
	})
	return params, err
}
