package keeper_test

import (
	"github.com/tendermint/tendermint/abci/types"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func (suite *KeeperTestSuite) TestEndBlock() {
	em := suite.ctx.EventManager()
	suite.Require().Equal(0, len(em.Events()))

	res := suite.app.EvmKeeper.EndBlock(suite.ctx, types.RequestEndBlock{})
	suite.Require().Equal([]types.ValidatorUpdate{}, res)

	// should emit 3 events on EndBlock: 1 coin spent, 1 burn, 1 block bloom
	suite.Require().Equal(3, len(em.Events()))
	suite.Require().Equal(banktypes.EventTypeCoinSpent, em.Events()[0].Type)
	suite.Require().Equal(banktypes.EventTypeCoinBurn, em.Events()[1].Type)
	suite.Require().Equal(evmtypes.EventTypeBlockBloom, em.Events()[2].Type)
}
