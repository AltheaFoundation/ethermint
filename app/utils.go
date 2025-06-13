// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/evmos/ethermint/blob/main/LICENSE
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/baseapp"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/cosmos/cosmos-sdk/testutil/mock"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	crisiskeeper "github.com/cosmos/cosmos-sdk/x/crisis/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	dbm "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/log"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/encoding"
	ethermint "github.com/evmos/ethermint/types"
	evmkeeper "github.com/evmos/ethermint/x/evm/keeper"
	feemarketkeeper "github.com/evmos/ethermint/x/feemarket/keeper"
)

// DefaultConsensusParams defines the default Tendermint consensus params used in
// EthermintApp testing.
var DefaultConsensusParams = &tmproto.ConsensusParams{
	Block: &tmproto.BlockParams{
		MaxBytes: 200000,
		MaxGas:   -1, // no limit
	},
	Evidence: &tmproto.EvidenceParams{
		MaxAgeNumBlocks: 302400,
		MaxAgeDuration:  504 * time.Hour, // 3 weeks is the max duration
		MaxBytes:        10000,
	},
	Validator: &tmproto.ValidatorParams{
		PubKeyTypes: []string{
			tmtypes.ABCIPubKeyTypeEd25519,
		},
	},
}

const TestChainId = "ethermint_9000-1"

// TestApp is a simple wrapper around an App. It exposes internal keepers for use in integration tests.
// This file also contains test helpers. Ideally they would be in separate package.
// Basic Usage:
//
//	Create a test app with NewTestApp, then all keepers and their methods can be accessed for test setup and execution.
//
// Advanced Usage:
//
//	Some tests call for an app to be initialized with some state. This can be achieved through keeper method calls (ie keeper.SetParams(...)).
//	However this leads to a lot of duplicated logic similar to InitGenesis methods.
//	So TestApp.InitializeFromGenesisStates() will call InitGenesis with the default genesis state.
//	and TestApp.InitializeFromGenesisStates(authState, cdpState) will do the same but overwrite the auth and cdp sections of the default genesis state
//	Creating the genesis states can be combersome, but helper methods can make it easier such as NewAuthGenStateFromAccounts below.
type TestApp struct {
	EthermintApp

	GenesisAddrs []sdk.AccAddress
}

const (
	// Bech32MainPrefix defines the Bech32 prefix for account addresses
	Bech32MainPrefix = "ethermint"
	// Bech32PrefixAccPub defines the Bech32 prefix of an account's public key
	Bech32PrefixAccPub = Bech32MainPrefix + "pub"
	// Bech32PrefixValAddr defines the Bech32 prefix of a validator's operator address
	Bech32PrefixValAddr = Bech32MainPrefix + "val" + "oper"
	// Bech32PrefixValPub defines the Bech32 prefix of a validator's operator public key
	Bech32PrefixValPub = Bech32MainPrefix + "val" + "oper" + "pub"
	// Bech32PrefixConsAddr defines the Bech32 prefix of a consensus node address
	Bech32PrefixConsAddr = Bech32MainPrefix + "val" + "cons"
	// Bech32PrefixConsPub defines the Bech32 prefix of a consensus node public key
	Bech32PrefixConsPub = Bech32MainPrefix + "val" + "cons" + "pub"

	Bip44CoinType = 459 // see https://github.com/satoshilabs/slips/blob/master/slip-0044.md
)

// NewTestApp creates a new TestApp
//
// Note, it also sets the sdk config with the app's address prefix, coin type, etc.
func NewTestApp() TestApp {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount(Bech32MainPrefix, Bech32PrefixAccPub)
	config.SetBech32PrefixForValidator(Bech32PrefixValAddr, Bech32PrefixValPub)
	config.SetBech32PrefixForConsensusNode(Bech32PrefixConsAddr, Bech32PrefixConsPub)
	config.SetCoinType(Bip44CoinType)

	return NewTestAppFromSealed()
}

// NewTestAppFromSealed creates a TestApp without first setting sdk config.
func NewTestAppFromSealed() TestApp {
	db := dbm.NewMemDB()

	encCfg := encoding.MakeConfig(ModuleBasics)

	app := NewEthermintApp(
		log.NewTMLogger(log.NewSyncWriter(os.Stdout)),
		db,
		nil,
		true,
		map[int64]bool{},
		DefaultNodeHome,
		5,
		encCfg,
		simtestutil.EmptyAppOptions{},
		baseapp.SetChainID("ethermint_9000-1"),
	)
	return TestApp{EthermintApp: *app}
}

// nolint
func (tApp TestApp) GetAccountKeeper() authkeeper.AccountKeeper { return tApp.AccountKeeper }
func (tApp TestApp) GetBankKeeper() bankkeeper.Keeper           { return tApp.BankKeeper }
func (tApp TestApp) GetMintKeeper() mintkeeper.Keeper           { return tApp.MintKeeper }
func (tApp TestApp) GetStakingKeeper() *stakingkeeper.Keeper    { return tApp.StakingKeeper }
func (tApp TestApp) GetSlashingKeeper() slashingkeeper.Keeper   { return tApp.SlashingKeeper }
func (tApp TestApp) GetDistrKeeper() distrkeeper.Keeper         { return tApp.DistrKeeper }
func (tApp TestApp) GetGovKeeper() govkeeper.Keeper             { return tApp.GovKeeper }
func (tApp TestApp) GetCrisisKeeper() crisiskeeper.Keeper       { return tApp.CrisisKeeper }
func (tApp TestApp) GetParamsKeeper() paramskeeper.Keeper       { return tApp.ParamsKeeper }
func (tApp TestApp) GetEvmKeeper() *evmkeeper.Keeper            { return tApp.EvmKeeper }
func (tApp TestApp) GetFeeMarketKeeper() feemarketkeeper.Keeper { return tApp.FeeMarketKeeper }

func (tApp TestApp) GetKVStoreKey(key string) *storetypes.KVStoreKey {
	return tApp.keys[key]
}

// Setup initializes a new EthermintApp. A Nop logger is set in EthermintApp.
func Setup(isCheckTx bool, patchGenesis func(*EthermintApp, ethermint.GenesisState) ethermint.GenesisState, t *testing.T) *EthermintApp {
	return SetupWithDB(isCheckTx, patchGenesis, dbm.NewMemDB(), t)
}

// SetupWithDB initializes a new EthermintApp. A Nop logger is set in EthermintApp.
func SetupWithDB(isCheckTx bool, patchGenesis func(*EthermintApp, ethermint.GenesisState) ethermint.GenesisState, db dbm.DB, t *testing.T) *EthermintApp {
	encCfg := encoding.MakeConfig(ModuleBasics)
	app := NewEthermintApp(
		log.NewTMLogger(log.NewSyncWriter(os.Stdout)),
		db,
		nil,
		true,
		map[int64]bool{},
		DefaultNodeHome,
		5,
		encCfg,
		simtestutil.EmptyAppOptions{},
		baseapp.SetChainID("ethermint_9000-1"),
	)

	genesisState := ModuleBasics.DefaultGenesis(encCfg.Codec)
	genesisState = NewTestGenesisState(&TestApp{EthermintApp: *app}, genesisState)

	if patchGenesis != nil {
		genesisState = patchGenesis(app, genesisState)
	}

	stateBytes, err := json.Marshal(genesisState)
	require.NoError(t, err)

	// init chain must be called to stop deliverState from being nil
	// Initialize the chain
	app.InitChain(
		abci.RequestInitChain{
			ChainId:         "ethermint_9000-1",
			InitialHeight:   1,
			ConsensusParams: simtestutil.DefaultConsensusParams,
			Validators:      nil,
			AppStateBytes:   stateBytes,
		},
	)

	return app
}

type GenesisState map[string]json.RawMessage

// NewTestGenesisState generate genesis state with single validator
func NewTestGenesisState(app *TestApp, genesisState GenesisState) GenesisState {
	privVal := mock.NewPV()
	pubKey, err := privVal.GetPubKey()
	if err != nil {
		panic(err)
	}
	// create validator set with single validator
	validator := tmtypes.NewValidator(pubKey, 1)
	valSet := tmtypes.NewValidatorSet([]*tmtypes.Validator{validator})

	// generate genesis account
	// generate genesis account
	senderPrivKey := secp256k1.GenPrivKey()
	acc := authtypes.NewBaseAccount(senderPrivKey.PubKey().Address().Bytes(), senderPrivKey.PubKey(), 0, 0)
	app.GenesisAddrs = append(app.GenesisAddrs, acc.GetAddress())

	balances := []banktypes.Balance{
		{
			Address: acc.GetAddress().String(),
			Coins:   sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(100000000000000))),
		},
	}

	return genesisStateWithValSet(app, genesisState, valSet, []authtypes.GenesisAccount{acc}, balances...)
}

func genesisStateWithValSet(app *TestApp, genesisState GenesisState,
	valSet *tmtypes.ValidatorSet, genAccs []authtypes.GenesisAccount,
	balances ...banktypes.Balance,
) GenesisState {
	// set genesis accounts
	authGenesis := authtypes.GetGenesisStateFromAppState(app.appCodec, genesisState)
	currentAccs, err := authtypes.UnpackAccounts(authGenesis.Accounts)
	if err != nil {
		panic(fmt.Errorf("error unpacking accounts: %w", err))
	}

	authGenesis = *authtypes.NewGenesisState(authGenesis.Params, append(currentAccs, genAccs...))

	validators := make([]stakingtypes.Validator, 0, len(valSet.Validators))
	delegations := make([]stakingtypes.Delegation, 0, len(valSet.Validators))

	bondAmt := sdk.DefaultPowerReduction

	for _, val := range valSet.Validators {
		pk, err := cryptocodec.FromTmPubKeyInterface(val.PubKey)
		if err != nil {
			panic(err)
		}
		pkAny, err := codectypes.NewAnyWithValue(pk)
		if err != nil {
			panic(err)
		}
		validator := stakingtypes.Validator{
			OperatorAddress:   sdk.ValAddress(val.Address).String(),
			ConsensusPubkey:   pkAny,
			Jailed:            false,
			Status:            stakingtypes.Bonded,
			Tokens:            bondAmt,
			DelegatorShares:   sdk.OneDec(),
			Description:       stakingtypes.Description{},
			UnbondingHeight:   int64(0),
			UnbondingTime:     time.Unix(0, 0).UTC(),
			Commission:        stakingtypes.NewCommission(sdk.ZeroDec(), sdk.ZeroDec(), sdk.ZeroDec()),
			MinSelfDelegation: sdk.ZeroInt(),
		}
		validators = append(validators, validator)
		delegations = append(delegations, stakingtypes.NewDelegation(genAccs[0].GetAddress(), val.Address.Bytes(), sdk.OneDec()))
	}
	// set validators and delegations
	stakingGenesis := stakingtypes.NewGenesisState(stakingtypes.DefaultParams(), validators, delegations)
	stakingGenesis.Validators = validators
	stakingGenesis.Params.UnbondingTime = time.Second * 1
	genesisState[stakingtypes.ModuleName] = app.appCodec.MustMarshalJSON(stakingGenesis)

	totalSupply := sdk.NewCoins()
	for _, b := range balances {
		// add genesis acc tokens to total supply
		totalSupply = totalSupply.Add(b.Coins...)
	}

	for range delegations {
		// add delegated tokens to total supply
		totalSupply = totalSupply.Add(sdk.NewCoin(sdk.DefaultBondDenom, bondAmt))
	}

	// add bonded amount to bonded pool module account
	balances = append(balances, banktypes.Balance{
		Address: authtypes.NewModuleAddress(stakingtypes.BondedPoolName).String(),
		Coins:   sdk.Coins{sdk.NewCoin(sdk.DefaultBondDenom, bondAmt)},
	})

	// update total supply
	bankGenesis := banktypes.NewGenesisState(banktypes.DefaultGenesisState().Params, balances, totalSupply, []banktypes.Metadata{}, []banktypes.SendEnabled{})
	bankGenesis.Balances = balances
	genesisState[banktypes.ModuleName] = app.appCodec.MustMarshalJSON(bankGenesis)

	return genesisState
}
