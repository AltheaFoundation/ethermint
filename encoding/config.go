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
package encoding

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	mtestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"

	enccodec "github.com/evmos/ethermint/encoding/codec"
)

// MakeConfig creates an EncodingConfig for testing
func MakeConfig(mb module.BasicManager) mtestutil.TestEncodingConfig {
	amino := codec.NewLegacyAmino()
	interfaceRegistry := types.NewInterfaceRegistry()
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txCfg := tx.NewTxConfig(marshaler, tx.DefaultSignModes)

	encodingConfig := mtestutil.TestEncodingConfig{
		InterfaceRegistry: interfaceRegistry,
		Codec:             marshaler,
		TxConfig:          txCfg,
		Amino:             amino,
	}

	enccodec.RegisterLegacyAminoCodec(encodingConfig.Amino)
	enccodec.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	mb.RegisterLegacyAminoCodec(encodingConfig.Amino)
	mb.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	return encodingConfig
}
