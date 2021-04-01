// Copyright 2020 IOTA Stiftung
// SPDX-License-Identifier: Apache-2.0

package chain

import (
	"fmt"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	txstream "github.com/iotaledger/goshimmer/packages/txstream/client"
	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/wasp/packages/coretypes"
	"github.com/iotaledger/wasp/packages/peering"
	"github.com/iotaledger/wasp/packages/tcrypto"
	"github.com/iotaledger/wasp/packages/vm/processors"
	"sync"
	"time"
)

type Committee interface {
	Chain() Chain // TODO temporary. Used for BlobCache access inside consensus. Not needed in the future
	Size() uint16
	Quorum() uint16
	OwnPeerIndex() uint16
	DKShare() *tcrypto.DKShare
	SendMsg(targetPeerIndex uint16, msgType byte, msgData []byte) error
	SendMsgToPeers(msgType byte, msgData []byte, ts int64) uint16
	IsAlivePeer(peerIndex uint16) bool
	QuorumIsAlive(quorum ...uint16) bool
	PeerStatus() []*PeerStatus
	OnPeerMessage(fun func(recv *peering.RecvEvent))
	Close()
	FeeDestination() coretypes.AgentID
}

// TODO temporary wrapper for Committee need replacement for all peers, not only committee.
//  Must be close to GroupProvider but less functions
type PeerGroupProvider interface {
	NumPeers() uint16
	NumIsAlive(quorum uint16) bool
	SendMsg(targetPeerIndex uint16, msgType byte, msgData []byte) error
	SendToAllUntilFirstError(msgType byte, msgData []byte) uint16
}

type Chain interface {
	Committee() Committee
	Mempool() Mempool
	ID() *coretypes.ChainID
	BlobCache() coretypes.BlobCache
	RegistryProvider() RegistryProvider

	// TODO distinguish external messages from internal and peer messages
	ReceiveMessage(interface{}) // generic
	ReceiveTransaction(*ledgerstate.Transaction)
	ReceiveInclusionState(ledgerstate.TransactionID, ledgerstate.InclusionState)
	ReceiveRequest(coretypes.Request)
	ReceiveState(stateOutput *ledgerstate.AliasOutput, timestamp time.Time)

	SetReadyStateManager() // TODO get rid
	SetReadyConsensus()    // TODO get rid
	Dismiss()
	IsDismissed() bool
	// requests
	GetRequestProcessingStatus(id coretypes.RequestID) RequestProcessingStatus
	EventRequestProcessed() *events.Event
	// chain processors
	Processors() *processors.ProcessorCache
}

type StateManager interface {
	SetPeers(PeerGroupProvider)
	EvidenceStateIndex(idx uint32)
	EventStateIndexPingPongMsg(msg *StateIndexPingPongMsg)
	EventGetBlockMsg(msg *GetBlockMsg)
	EventBlockHeaderMsg(msg *BlockHeaderMsg)
	EventStateUpdateMsg(msg *StateUpdateMsg)
	EventStateOutputMsg(msg *StateMsg)
	EventPendingBlockMsg(msg PendingBlockMsg)
	EventTimerMsg(msg TimerTick)
	Close()
}

type Consensus interface {
	EventStateTransitionMsg(*StateTransitionMsg)
	EventRequestMsg(coretypes.Request)
	EventNotifyReqMsg(*NotifyReqMsg)
	EventStartProcessingBatchMsg(*StartProcessingBatchMsg)
	EventResultCalculated(msg *VMResultMsg)
	EventSignedHashMsg(*SignedHashMsg)
	EventNotifyFinalResultPostedMsg(*NotifyFinalResultPostedMsg)
	EventTransactionInclusionStateMsg(msg *InclusionStateMsg)
	EventTimerMsg(TimerTick)
	Close()
	//
	IsRequestInBacklog(coretypes.RequestID) bool
}

type ReadyListRecord struct {
	Request coretypes.Request
	Seen    map[uint16]bool
}

type Mempool interface {
	ReceiveRequest(req coretypes.Request)
	MarkSeenByCommitteePeer(reqid *coretypes.RequestID, peerIndex uint16)
	ClearSeenMarks()
	GetReadyList(seenThreshold uint16) []coretypes.Request
	GetReadyListFull(seenThreshold uint16) []*ReadyListRecord
	TakeAllReady(nowis time.Time, reqids ...coretypes.RequestID) ([]coretypes.Request, bool)
	RemoveRequests(reqs ...coretypes.RequestID)
	HasRequest(id coretypes.RequestID) bool
	Close()
}

type PeerStatus struct {
	Index     int
	PeeringID string
	IsSelf    bool
	Connected bool
}

func (p *PeerStatus) String() string {
	return fmt.Sprintf("%+v", *p)
}

type RequestProcessingStatus int

const (
	RequestProcessingStatusUnknown = RequestProcessingStatus(iota)
	RequestProcessingStatusBacklog
	RequestProcessingStatusCompleted
)

type chainConstructor func(
	chr *ChainRecord,
	log *logger.Logger,
	nodeConn *txstream.Client,
	netProvider peering.NetworkProvider,
	dksProvider tcrypto.RegistryProvider,
	blobProvider coretypes.BlobCache,
	rProvider RegistryProvider,
	onActivation func(),
) Chain

var constructorNew chainConstructor
var constructorNewMutex sync.Mutex

func RegisterChainConstructor(constr chainConstructor) {
	constructorNewMutex.Lock()
	defer constructorNewMutex.Unlock()

	if constructorNew != nil {
		panic("RegisterChainConstructor: already registered")
	}
	constructorNew = constr
}

func New(
	chr *ChainRecord,
	log *logger.Logger,
	nodeConn *txstream.Client,
	netProvider peering.NetworkProvider,
	dksProvider tcrypto.RegistryProvider,
	blobProvider coretypes.BlobCache,
	rProvider RegistryProvider,
	onActivation func(),
) Chain {
	return constructorNew(chr, log, nodeConn, netProvider, dksProvider, blobProvider, rProvider, onActivation)
}

type mempoolConstructor func(cache coretypes.BlobCache) Mempool

var mempoolConstructorFun mempoolConstructor
var mempoolConstructorFunMutex sync.Mutex

func RegisterMempoollConstructor(constr mempoolConstructor) {
	mempoolConstructorFunMutex.Lock()
	defer mempoolConstructorFunMutex.Unlock()

	if mempoolConstructorFun != nil {
		panic("RegistermempoolConstructor: already registered")
	}
	mempoolConstructorFun = constr
}

func NewMempool(blobCache coretypes.BlobCache) Mempool {
	return mempoolConstructorFun(blobCache)
}
