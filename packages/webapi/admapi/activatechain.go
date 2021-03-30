package admapi

import (
	"fmt"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"net/http"

	"github.com/iotaledger/wasp/packages/coretypes"
	"github.com/iotaledger/wasp/packages/webapi/httperrors"
	"github.com/iotaledger/wasp/packages/webapi/routes"
	"github.com/iotaledger/wasp/plugins/chains"
	"github.com/iotaledger/wasp/plugins/registry"
	"github.com/labstack/echo/v4"
	"github.com/pangpanglabs/echoswagger/v2"
)

func addChainEndpoints(adm echoswagger.ApiGroup) {
	adm.POST(routes.ActivateChain(":chainID"), handleActivateChain).
		AddParamPath("", "chainID", "ChainID (base58)").
		SetSummary("Activate a chain")

	adm.POST(routes.DeactivateChain(":chainID"), handleDeactivateChain).
		AddParamPath("", "chainID", "ChainID (base58)").
		SetSummary("Deactivate a chain")
}

func handleActivateChain(c echo.Context) error {
	scAddress, err := ledgerstate.AddressFromBase58EncodedString(c.Param("chainID"))
	if err != nil {
		return httperrors.BadRequest(fmt.Sprintf("Invalid SC address: %s", c.Param("address")))
	}
	chainID, err := coretypes.ChainIDFromAddress(scAddress)
	if err != nil {
		return err
	}
	registry := registry.DefaultRegistry()
	bd, err := registry.ActivateChainRecord(chainID)
	if err != nil {
		return err
	}

	log.Debugw("calling committees.Activate", "chainID", bd.ChainID.String())
	if err := chains.AllChains().Activate(bd, registry); err != nil {
		return err
	}

	return c.NoContent(http.StatusOK)
}

func handleDeactivateChain(c echo.Context) error {
	scAddress, err := ledgerstate.AddressFromBase58EncodedString(c.Param("chainID"))
	if err != nil {
		return httperrors.BadRequest(fmt.Sprintf("Invalid chain id: %s", c.Param("chainID")))
	}
	chainID, err := coretypes.ChainIDFromAddress(scAddress)
	if err != nil {
		return err
	}
	bd, err := registry.DefaultRegistry().DeactivateChainRecord(chainID)
	if err != nil {
		return err
	}

	err = chains.AllChains().Deactivate(bd)
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusOK)
}
