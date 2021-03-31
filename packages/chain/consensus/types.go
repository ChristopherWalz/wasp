// Copyright 2020 IOTA Stiftung
// SPDX-License-Identifier: Apache-2.0

package consensus

import (
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	txstream "github.com/iotaledger/goshimmer/packages/txstream/client"
	"sync"
	"time"

	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/wasp/packages/chain"
	"github.com/iotaledger/wasp/packages/coretypes"
	"github.com/iotaledger/wasp/packages/dbprovider"
	"github.com/iotaledger/wasp/packages/hashing"
	"github.com/iotaledger/wasp/packages/state"
	"github.com/iotaledger/wasp/packages/tcrypto/tbdn"
	"github.com/iotaledger/wasp/packages/util"
)

type operator struct {
	committee chain.Committee
	nodeConn  *txstream.Client
	//currentState
	currentState   state.VirtualState
	stateOutput    *ledgerstate.AliasOutput
	stateTimestamp time.Time

	// consensus stage
	consensusStage         int
	consensusStageDeadline time.Time
	//
	requestBalancesDeadline time.Time

	// notifications with future currentState indices
	notificationsBacklog []*chain.NotifyReqMsg

	// backlog of requests with all information
	requests map[coretypes.RequestID]*request

	peerPermutation *util.Permutation16

	leaderStatus            *leaderStatus
	sentResultToLeaderIndex uint16
	sentResultToLeader      *ledgerstate.TransactionEssence

	postedResultTxid       ledgerstate.TransactionID
	nextPullInclusionLevel time.Time // if postedResultTxid != nil

	nextArgSolidificationDeadline time.Time

	log *logger.Logger

	// data for concurrent access, from APIs mostly
	concurrentAccessMutex sync.RWMutex
	requestIdsProtected   map[coretypes.RequestID]bool

	// Channels for accepting external events.
	eventStateTransitionMsgCh           chan *chain.StateTransitionMsg
	eventRequestMsgCh                   chan coretypes.Request
	eventNotifyReqMsgCh                 chan *chain.NotifyReqMsg
	eventStartProcessingBatchMsgCh      chan *chain.StartProcessingBatchMsg
	eventResultCalculatedCh             chan *chain.VMResultMsg
	eventSignedHashMsgCh                chan *chain.SignedHashMsg
	eventNotifyFinalResultPostedMsgCh   chan *chain.NotifyFinalResultPostedMsg
	eventTransactionInclusionLevelMsgCh chan *chain.InclusionStateMsg
	eventTimerMsgCh                     chan chain.TimerTick
	closeCh                             chan bool
	dbProvider                          *dbprovider.DBProvider
}

type leaderStatus struct {
	reqs            []*request
	batch           state.Block
	batchHash       hashing.HashValue
	timestamp       time.Time
	resultTxEssence *ledgerstate.TransactionEssence
	finalized       bool
	signedResults   []*signedResult
}

type signedResult struct {
	essenceHash hashing.HashValue
	sigShare    tbdn.SigShare
}

// backlog entry. Keeps stateOutput of the request
type request struct {
	req             coretypes.Request
	whenMsgReceived time.Time
	// notification vector for the current state
	notifications []bool
	// true if arguments were decoded/solidified already. If not, the request in not eligible for the batch
	argsSolid bool

	log *logger.Logger
}

func New(committee chain.Committee, nodeConn *txstream.Client, log *logger.Logger, dbProvider *dbprovider.DBProvider) *operator {
	ret := &operator{
		committee:                           committee,
		nodeConn:                            nodeConn,
		requests:                            make(map[coretypes.RequestID]*request),
		requestIdsProtected:                 make(map[coretypes.RequestID]bool),
		peerPermutation:                     util.NewPermutation16(committee.Size(), nil),
		log:                                 log.Named("c"),
		eventStateTransitionMsgCh:           make(chan *chain.StateTransitionMsg),
		eventRequestMsgCh:                   make(chan coretypes.Request),
		eventNotifyReqMsgCh:                 make(chan *chain.NotifyReqMsg),
		eventStartProcessingBatchMsgCh:      make(chan *chain.StartProcessingBatchMsg),
		eventResultCalculatedCh:             make(chan *chain.VMResultMsg),
		eventSignedHashMsgCh:                make(chan *chain.SignedHashMsg),
		eventNotifyFinalResultPostedMsgCh:   make(chan *chain.NotifyFinalResultPostedMsg),
		eventTransactionInclusionLevelMsgCh: make(chan *chain.InclusionStateMsg),
		eventTimerMsgCh:                     make(chan chain.TimerTick),
		closeCh:                             make(chan bool),
		dbProvider:                          dbProvider,
	}
	ret.setNextConsensusStage(consensusStageNoSync)
	go ret.recvLoop()
	return ret
}

func (op *operator) Close() {
	close(op.closeCh)
}

func (op *operator) recvLoop() {
	for {
		select {
		case msg, ok := <-op.eventStateTransitionMsgCh:
			if ok {
				op.eventStateTransitionMsg(msg)
			}
		case msg, ok := <-op.eventRequestMsgCh:
			if ok {
				op.eventRequestMsg(msg)
			}
		case msg, ok := <-op.eventNotifyReqMsgCh:
			if ok {
				op.eventNotifyReqMsg(msg)
			}
		case msg, ok := <-op.eventStartProcessingBatchMsgCh:
			if ok {
				op.eventStartProcessingBatchMsg(msg)
			}
		case msg, ok := <-op.eventResultCalculatedCh:
			if ok {
				op.eventResultCalculated(msg)
			}
		case msg, ok := <-op.eventSignedHashMsgCh:
			if ok {
				op.eventSignedHashMsg(msg)
			}
		case msg, ok := <-op.eventNotifyFinalResultPostedMsgCh:
			if ok {
				op.eventNotifyFinalResultPostedMsg(msg)
			}
		case msg, ok := <-op.eventTransactionInclusionLevelMsgCh:
			if ok {
				op.eventTransactionInclusionStateMsg(msg)
			}
		case msg, ok := <-op.eventTimerMsgCh:
			if ok {
				op.eventTimerMsg(msg)
			}
		case <-op.closeCh:
			return
		}
	}
}

func (op *operator) peerIndex() uint16 {
	return op.committee.OwnPeerIndex()
}

func (op *operator) quorum() uint16 {
	return op.committee.Quorum()
}

func (op *operator) size() uint16 {
	return op.committee.Size()
}

func (op *operator) blockIndex() (uint32, bool) {
	if op.currentState == nil {
		return 0, false
	}
	return op.currentState.BlockIndex(), true
}

func (op *operator) mustStateIndex() uint32 {
	ret, ok := op.blockIndex()
	if !ok {
		panic("mustStateIndex")
	}
	return ret
}

func (op *operator) getFeeDestination() coretypes.AgentID {
	return op.committee.FeeDestination()
}
