package mocknode

import (
	"testing"
	"time"

	"github.com/iotaledger/goshimmer/client"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/stretchr/testify/require"
)

func TestMockNode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mocknode test in short mode")
	}

	initOk := make(chan (bool))
	m := Start(":5000", ":8080", initOk)
	defer m.Stop()

	select {
	case result := <-initOk:
		require.True(t, result)
	case <-time.After(1 * time.Second):
		t.Fatalf("Timeout waiting for mocknode start")
	}

	_, addr := m.Ledger.NewKeyPairByIndex(2)

	cl := client.NewGoShimmerAPI("http://127.0.0.1:8080")
	_, err := cl.SendFaucetRequest(addr.Base58())
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	r, err := cl.GetAddressUnspentOutputs(addr.Base58())
	require.NoError(t, err)

	require.Equal(t, 1, len(r.Outputs))
	out, err := r.Outputs[0].ToLedgerstateOutput()
	require.NoError(t, err)
	require.Equal(t, addr.Base58(), out.Address().Base58())
	require.Equal(t, 1, out.Balances().Size())
	b, _ := out.Balances().Get(ledgerstate.ColorIOTA)
	require.EqualValues(t, 1337, b)
}
