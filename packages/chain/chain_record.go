package chain

import (
	"fmt"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/hive.go/marshalutil"
	"github.com/iotaledger/wasp/packages/coretypes"
)

// ChainRecord represents chain the node is participating in
// TODO optimize, no need for a persistent structure, simple activity tag is enough
type ChainRecord struct {
	ChainID coretypes.ChainID
	Active  bool
}

func NewChainRecord(chainID *coretypes.ChainID, active ...bool) *ChainRecord {
	act := false
	if len(active) > 0 {
		act = active[0]
	}
	return &ChainRecord{
		ChainID: chainID.Clone(),
		Active:  act,
	}
}

func ChainRecordFromMarshalUtil(mu *marshalutil.MarshalUtil) (*ChainRecord, error) {
	ret := &ChainRecord{}
	aliasAddr, err := ledgerstate.AliasAddressFromMarshalUtil(mu)
	if err != nil {
		return nil, err
	}
	ret.ChainID = *coretypes.NewChainID(aliasAddr)

	ret.Active, err = mu.ReadBool()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// CommitteeRecordFromBytes
func ChainRecordFromBytes(data []byte) (*ChainRecord, error) {
	return ChainRecordFromMarshalUtil(marshalutil.New(data))
}

func (rec *ChainRecord) Bytes() []byte {
	return marshalutil.New().
		WriteBytes(rec.ChainID.Bytes()).
		WriteBool(rec.Active).
		Bytes()
}

func (rec *ChainRecord) String() string {
	ret := "ChainID: " + rec.ChainID.String() + "\n"
	ret += fmt.Sprintf("      Active: %v\n", rec.Active)
	return ret
}
