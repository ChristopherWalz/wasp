package runvm

import (
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/packages/ledgerstate/utxoutil"
	"github.com/iotaledger/wasp/packages/coretypes"
	"github.com/iotaledger/wasp/packages/hashing"
	"github.com/iotaledger/wasp/packages/kv/dict"
	"github.com/iotaledger/wasp/packages/state"
	"github.com/iotaledger/wasp/packages/util"
	"github.com/iotaledger/wasp/packages/vm"
	"github.com/iotaledger/wasp/packages/vm/vmcontext"
	"golang.org/x/xerrors"
)

// MustRunComputationsAsync runs computations for the batch of requests in the background
// This is the main entry point to the VM
// TODO timeout for VM. Gas limit
func MustRunComputationsAsync(ctx *vm.VMTask) {
	if len(ctx.Requests) == 0 {
		ctx.Log.Panicf("MustRunComputationsAsync: must be at least 1 request")
	}
	outputs := outputsFromRequests(ctx.Requests...)
	txb := utxoutil.NewBuilder(append(outputs, ctx.ChainInput)...)

	go runTask(ctx, txb)
}

// runTask runs batch of requests on VM
func runTask(task *vm.VMTask, txb *utxoutil.Builder) {
	task.Log.Debugw("runTask IN",
		"chainID", task.ChainInput.Address().Base58(),
		"timestamp", task.Timestamp,
		"block index", task.VirtualState.BlockIndex(),
		"num req", len(task.Requests),
	)
	vmctx, err := vmcontext.MustNewVMContext(task, txb)
	if err != nil {
		task.Log.Panicf("runTask: %v", err)
	}

	stateUpdates := make([]state.StateUpdate, 0, len(task.Requests))
	var lastResult dict.Dict
	var lastErr error
	var lastStateUpdate state.StateUpdate
	var lastTotalAssets *ledgerstate.ColoredBalances

	// loop over the batch of requests and run each request on the VM.
	// the result accumulates in the VMContext and in the list of stateUpdates
	for i, req := range task.Requests {
		vmctx.RunTheRequest(req, i)
		lastStateUpdate, lastResult, lastTotalAssets, lastErr = vmctx.GetResult()

		stateUpdates = append(stateUpdates, lastStateUpdate)
	}

	// create block from state updates.
	task.ResultBlock, err = state.NewBlock(stateUpdates...)
	if err != nil {
		task.OnFinish(nil, nil, xerrors.Errorf("RunVM.NewBlock: %v", err))
		return
	}
	task.ResultBlock.WithBlockIndex(task.VirtualState.BlockIndex() + 1)

	// calculate resulting state hash
	vsClone := task.VirtualState.Clone()
	if err = vsClone.ApplyBlock(task.ResultBlock); err != nil {
		task.OnFinish(nil, nil, xerrors.Errorf("RunVM.ApplyBlock: %v", err))
		return
	}

	task.ResultTransaction, err = vmctx.BuildTransactionEssence(vsClone.Hash())
	if err != nil {
		task.OnFinish(nil, nil, xerrors.Errorf("RunVM.BuildTransactionEssence: %v", err))
		return
	}
	chainOutput, err := utxoutil.GetSingleChainedAliasOutput(task.ResultTransaction)
	if err != nil {
		task.OnFinish(nil, nil, xerrors.Errorf("RunVM.BuildTransactionEssence: %v", err))
		return
	}
	diffAssets := util.DiffColoredBalances(chainOutput.Balances(), lastTotalAssets)
	if iotas, ok := diffAssets[ledgerstate.ColorIOTA]; !ok || iotas != ledgerstate.DustThresholdAliasOutputIOTA {
		task.OnFinish(nil, nil, xerrors.Errorf("RunVM.BuildTransactionEssence: inconsistency between L1 and L2 ledgers"))
		return
	}

	task.Log.Debugw("runTask OUT",
		"batch size", task.ResultBlock.Size(),
		"block index", task.ResultBlock.StateIndex(),
		"variable state hash", vsClone.Hash().Bytes(),
		"tx essence hash", hashing.HashData(task.ResultTransaction.Bytes()).String(),
		"tx finalTimestamp", task.ResultTransaction.Timestamp(),
	)
	task.OnFinish(lastResult, lastErr, nil)
}

// outputsFromRequests collect all outputs from requests which are on-ledger
func outputsFromRequests(requests ...coretypes.Request) []ledgerstate.Output {
	ret := make([]ledgerstate.Output, 0)
	for _, req := range requests {
		if out := req.Output(); out != nil {
			ret = append(ret, out)
		}
	}
	return ret
}
