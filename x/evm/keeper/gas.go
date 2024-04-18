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
package keeper

import (
	"math/big"

	errorsmod "cosmossdk.io/errors"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/evmos/ethermint/x/evm/types"
)

// GetEthIntrinsicGas returns the intrinsic gas cost for the transaction
func (k *Keeper) GetEthIntrinsicGas(ctx sdk.Context, msg core.Message, cfg *params.ChainConfig, isContractCreation bool) (uint64, error) {
	height := big.NewInt(ctx.BlockHeight())
	homestead := cfg.IsHomestead(height)
	istanbul := cfg.IsIstanbul(height)

	return core.IntrinsicGas(msg.Data(), msg.AccessList(), isContractCreation, homestead, istanbul)
}

// RefundExcessGas transfers the leftover gas to the sender of the message
// Additionally, the function sets the total gas consumed to the value
// returned by the EVM execution, thus ignoring the previous intrinsic gas consumed during in the
// AnteHandler.
// The remaining gas will be burnt in the EndBlocker
func (k *Keeper) RefundExcessGas(ctx sdk.Context, msg core.Message, leftoverGas uint64, denom string) error {
	// Return EVM tokens for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(leftoverGas), msg.GasPrice())
	gasAccountBalance := k.bankKeeper.GetBalance(ctx, types.FeeBurnerAccount, denom)

	switch remaining.Sign() {
	case -1:
		// negative refund errors
		return errorsmod.Wrapf(types.ErrInvalidRefund, "refunded amount value cannot be negative %d", remaining.Int64())
	case 1:
		// positive amount refund
		refundCoin := sdk.NewCoin(denom, sdk.NewIntFromBigInt(remaining))
		if gasAccountBalance.IsLT(refundCoin) {
			return errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "fee burner account has insufficient funds (%s) to refund %s", gasAccountBalance.String(), refundCoin.String())
		}

		// refund to sender from the fee burner account, which is the escrow account in charge of collecting and burning EVM tx fees

		err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.FeeBurner, msg.From().Bytes(), sdk.NewCoins(refundCoin))
		if err != nil {
			return errorsmod.Wrapf(err, "failed to refund %d leftover gas (%s)", leftoverGas, refundCoin.String())
		}
	default:
		// no refund, consume gas and update the tx gas meter
	}

	return nil
}

func (k *Keeper) BurnConsumedGas(ctx sdk.Context) error {
	denom := k.GetParams(ctx).EvmDenom
	gasAccountBalance := k.bankKeeper.GetBalance(ctx, types.FeeBurnerAccount, denom)
	if err := k.bankKeeper.BurnCoins(ctx, types.FeeBurner, sdk.NewCoins(gasAccountBalance)); err != nil {
		return errorsmod.Wrap(err, "failed to burn FeeBurner account balance")
	}
	return nil
}

// ResetGasMeterAndConsumeGas reset first the gas meter consumed value to zero and set it back to the new value
// 'gasUsed'
func (k *Keeper) ResetGasMeterAndConsumeGas(ctx sdk.Context, gasUsed uint64) {
	// reset the gas count
	ctx.GasMeter().RefundGas(ctx.GasMeter().GasConsumed(), "reset the gas count")
	ctx.GasMeter().ConsumeGas(gasUsed, "apply evm transaction")
}

// GasToRefund calculates the amount of gas the state machine should refund to the sender. It is
// capped by the refund quotient value.
// Note: do not pass 0 to refundQuotient
func GasToRefund(availableRefund, gasConsumed, refundQuotient uint64) uint64 {
	// Apply refund counter
	refund := gasConsumed / refundQuotient
	if refund > availableRefund {
		return availableRefund
	}
	return refund
}
