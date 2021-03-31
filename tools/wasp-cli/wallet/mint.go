package wallet

import (
	"os"
	"strconv"

	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/packages/ledgerstate/utxoutil"
	"github.com/iotaledger/wasp/tools/wasp-cli/config"
	"github.com/iotaledger/wasp/tools/wasp-cli/log"
	"github.com/iotaledger/wasp/tools/wasp-cli/util"
)

func mintCmd(args []string) {
	if len(args) < 1 {
		log.Usage("%s mint <amount>\n", os.Args[0])
	}

	amount, err := strconv.Atoi(args[0])
	log.Check(err)

	wallet := Load()
	address := wallet.Address()

	outs, err := config.GoshimmerClient().GetConfirmedOutputs(address)
	log.Check(err)

	tx := util.WithTransaction(func() (*ledgerstate.Transaction, error) {
		txb := utxoutil.NewBuilder(outs...)
		log.Check(txb.AddSigLockedColoredOutput(address, nil, uint64(amount)))
		log.Check(txb.AddReminderOutputIfNeeded(address, nil, true))
		return txb.BuildWithED25519(wallet.KeyPair())

	})

	log.Printf("Minted %d tokens of color %s\n", amount, tx.ID())
}
