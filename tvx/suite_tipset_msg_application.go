package main

import (
	"os"

	address "github.com/filecoin-project/go-address"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/hashicorp/go-multierror"
	"github.com/urfave/cli/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
	"github.com/filecoin-project/oni/tvx/schema"
)

var suiteTipsetsCmd = &cli.Command{
	Name:        "suite-tipsets",
	Description: "generate test vectors from the tipsets test suite adapted from github.com/filecoin-project/chain-validation",
	Action:      suiteTipsets,
}

func suiteTipsets(c *cli.Context) error {
	var err *multierror.Error
	err = multierror.Append(TipSetTest_BlockMessageApplication())
	return err.ErrorOrNil()
}

func TipSetTest_BlockMessageApplication() error {
	const gasLimit = 1000000000

	err := func(testname string) error {
		td := drivers.NewTestDriver(schema.NewTipsetTestVector())
		td.Vector.Meta.Desc = testname

		tipB := drivers.NewTipSetMessageBuilder(td)
		blkB := drivers.NewBlockBuilder(td, td.ExeCtx.Miner)

		senderBLS, _ := td.NewAccountActor(address.BLS, big_spec.NewInt(10*gasLimit))
		receiverBLS, _ := td.NewAccountActor(address.BLS, big_spec.Zero())
		senderSECP, _ := td.NewAccountActor(address.SECP256K1, big_spec.NewInt(10*gasLimit))
		receiverSECP, _ := td.NewAccountActor(address.SECP256K1, big_spec.Zero())
		transferAmnt := abi_spec.NewTokenAmount(100)

		td.UpdatePreStateRoot()

		results := tipB.WithBlockBuilder(
			blkB.
				WithBLSMessageOk(
					td.MessageProducer.Transfer(senderBLS, receiverBLS, chain.Nonce(0), chain.Value(transferAmnt))).
				WithSECPMessageOk(
					td.MessageProducer.Transfer(senderSECP, receiverSECP, chain.Nonce(0), chain.Value(transferAmnt))),
		).ApplyAndValidate()

		require.Equal(drivers.T, 2, len(results.Receipts))

		blsGasUsed := int64(results.Receipts[0].GasUsed)
		secpGasUsed := int64(results.Receipts[1].GasUsed)
		assert.Greater(drivers.T, secpGasUsed, blsGasUsed)

		td.MustSerialize(os.Stdout)

		return nil
	}("SECP and BLS messages cost different amounts of gas")
	if err != nil {
		return err
	}

	return nil
}
