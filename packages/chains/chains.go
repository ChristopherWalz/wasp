package chains

import (
	txstream "github.com/iotaledger/goshimmer/packages/txstream/client"
	"sync"

	"golang.org/x/xerrors"

	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/wasp/packages/chain"
	"github.com/iotaledger/wasp/packages/coretypes"
	registry_pkg "github.com/iotaledger/wasp/packages/registry"
	"github.com/iotaledger/wasp/plugins/peering"
	"github.com/iotaledger/wasp/plugins/registry"
)

type Chains struct {
	mutex     sync.RWMutex
	log       *logger.Logger
	allChains map[[ledgerstate.AddressLength]byte]chain.Chain
	nodeConn  *txstream.Client
}

func New(log *logger.Logger) *Chains {
	ret := &Chains{
		log:       log,
		allChains: make(map[[ledgerstate.AddressLength]byte]chain.Chain),
	}
	return ret
}

func (c *Chains) Dismiss() {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	for _, ch := range c.allChains {
		ch.Dismiss()
	}
	c.allChains = make(map[[ledgerstate.AddressLength]byte]chain.Chain)
}

func (c *Chains) Attach(nodeConn *txstream.Client) {
	if c.nodeConn != nil {
		c.log.Panicf("Chains: already attached")
	}
	c.nodeConn = nodeConn
	c.nodeConn.Events.TransactionReceived.Attach(events.NewClosure(c.dispatchMsgTransaction))
	c.nodeConn.Events.InclusionStateReceived.Attach(events.NewClosure(c.dispatchMsgInclusionState))
	// TODO attach to off-ledger request module
}

func (c *Chains) ActivateAllFromRegistry() error {
	chainRecords, err := registry_pkg.GetChainRecords()
	if err != nil {
		return err
	}

	astr := make([]string, len(chainRecords))
	for i := range astr {
		astr[i] = chainRecords[i].ChainID.String()[:10] + ".."
	}
	c.log.Debugf("loaded %d chain record(s) from registry: %+v", len(chainRecords), astr)

	for _, chr := range chainRecords {
		if chr.Active {
			if err := c.Activate(chr); err != nil {
				c.log.Errorf("cannot activate chain %s: %v", chr.ChainID, err)
			}
		}
	}
	return nil
}

// Activate activates chain on the Wasp node:
// - creates chain object
// - insert it into the runtime registry
// - subscribes for related transactions in he IOTA node
func (c *Chains) Activate(chr *registry_pkg.ChainRecord) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !chr.Active {
		return xerrors.Errorf("cannot activate chain for deactivated chain record")
	}
	chainArr := chr.ChainID.Array()
	_, ok := c.allChains[chainArr]
	if ok {
		c.log.Debugf("chain is already active: %s", chr.ChainID.String())
		return nil
	}
	// create new chain object
	defaultRegistry := registry.DefaultRegistry()
	ch := chain.New(chr, c.log, c.nodeConn, peering.DefaultNetworkProvider(), defaultRegistry, defaultRegistry, func() {
		c.nodeConn.Subscribe(chr.ChainID.AliasAddress)
	})
	if ch != nil {
		c.allChains[chainArr] = ch
		c.log.Infof("activated chain:\n%s", chr.String())
	} else {
		c.log.Infof("failed to activate chain:\n%s", chr.String())
	}
	return nil
}

// Deactivate deactivates chain in the node
func (c *Chains) Deactivate(chr *registry_pkg.ChainRecord) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ch, ok := c.allChains[chr.ChainID.Array()]
	if !ok || ch.IsDismissed() {
		c.log.Debugf("chain is not active: %s", chr.ChainID.String())
		return nil
	}
	ch.Dismiss()
	c.nodeConn.Unsubscribe(chr.ChainID.AliasAddress)
	c.log.Debugf("chain has been deactivated: %s", chr.ChainID.String())
	return nil
}

// Get returns active chain object or nil if it doesn't exist
// lazy unsubscribing
func (c *Chains) Get(chainID *coretypes.ChainID) chain.Chain {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	addrArr := chainID.Array()
	ret, ok := c.allChains[addrArr]
	if ok && ret.IsDismissed() {
		delete(c.allChains, addrArr)
		c.nodeConn.Unsubscribe(chainID.AliasAddress)
		return nil
	}
	return ret
}
