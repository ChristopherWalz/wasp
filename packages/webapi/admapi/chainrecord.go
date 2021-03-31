package admapi

import (
	"fmt"
	"net/http"

	"github.com/iotaledger/wasp/packages/coretypes"
	"github.com/iotaledger/wasp/packages/webapi/httperrors"
	"github.com/iotaledger/wasp/packages/webapi/model"
	"github.com/iotaledger/wasp/packages/webapi/routes"
	"github.com/iotaledger/wasp/plugins/registry"
	"github.com/labstack/echo/v4"
	"github.com/pangpanglabs/echoswagger/v2"
)

func addChainRecordEndpoints(adm echoswagger.ApiGroup) {
	rnd1 := coretypes.RandomChainID()
	example := model.ChainRecord{
		ChainID: model.NewChainID(rnd1),
		Active:  false,
	}

	adm.POST(routes.PutChainRecord(), handlePutChainRecord).
		SetSummary("Create a new chain record").
		AddParamBody(example, "Record", "Chain record", true)

	adm.GET(routes.GetChainRecord(":chainID"), handleGetChainRecord).
		SetSummary("Find the chain record for the given chain ID").
		AddParamPath("", "chainID", "ChainID (base58)").
		AddResponse(http.StatusOK, "Chain Record", example, nil)

	adm.GET(routes.ListChainRecords(), handleGetChainRecordList).
		SetSummary("Get the list of chain records in the node").
		AddResponse(http.StatusOK, "Chain Record", []model.ChainRecord{example}, nil)
}

func handlePutChainRecord(c echo.Context) error {
	var req model.ChainRecord

	if err := c.Bind(&req); err != nil {
		return httperrors.BadRequest("Invalid request body")
	}

	bd := req.Record()

	registry := registry.DefaultRegistry()
	bd2, err := registry.GetChainRecord(&bd.ChainID)
	if err != nil {
		return err
	}
	if bd2 != nil {
		return httperrors.Conflict(fmt.Sprintf("Record already exists: %s", bd.ChainID.String()))
	}
	if err = registry.SaveChainRecord(bd); err != nil {
		return err
	}

	log.Infof("Record saved. ChainID: %s", bd.ChainID.String())

	return c.NoContent(http.StatusCreated)
}

func handleGetChainRecord(c echo.Context) error {
	chainID, err := coretypes.ChainIDFromBase58(c.Param("chainID"))
	if err != nil {
		return httperrors.BadRequest(err.Error())
	}
	bd, err := registry.DefaultRegistry().GetChainRecord(chainID)
	if err != nil {
		return err
	}
	if bd == nil {
		return httperrors.NotFound(fmt.Sprintf("Record not found: %s", chainID))
	}
	return c.JSON(http.StatusOK, model.NewChainRecord(bd))
}

func handleGetChainRecordList(c echo.Context) error {
	lst, err := registry.DefaultRegistry().GetChainRecords()
	if err != nil {
		return err
	}
	ret := make([]*model.ChainRecord, len(lst))
	for i := range ret {
		ret[i] = model.NewChainRecord(lst[i])
	}
	return c.JSON(http.StatusOK, ret)
}
