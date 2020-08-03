package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"
)

var execLotusCmd = &cli.Command{
	Name:        "exec-lotus",
	Description: "execute a test vector against Lotus",
	Flags:       []cli.Flag{&fileFlag},
	Action:      runExecLotus,
}

func runExecLotus(c *cli.Context) error {
	f := c.String("file")
	if f == "" {
		return fmt.Errorf("test vector file cannot be empty")
	}

	file, err := os.Open(f)
	if err != nil {
		return fmt.Errorf("failed to open test vector: %w", err)
	}

	var (
		dec = json.NewDecoder(file)
		tv  TestVector
	)

	if err = dec.Decode(&tv); err != nil {
		return fmt.Errorf("failed to decode test vector: %w", err)
	}

	switch tv.Class {
	case "messages":
		var (
			ctx   = context.TODO()
			epoch = tv.Pre.Epoch
			root  = tv.Pre.StateTree.RootCID
		)

		bs := blockstore.NewBlockstore(datastore.NewMapDatastore())

		header, err := car.LoadCar(bs, bytes.NewReader(tv.Pre.StateTree.CAR))
		if err != nil {
			return fmt.Errorf("failed to load state tree car from test vector: %w", err)
		}

		fmt.Println(header.Roots)

		cst := cbor.NewCborStore(bs)
		rootCid, err := cid.Parse(root)
		if err != nil {
			return err
		}

		// Load the state tree.
		_, err = state.LoadStateTree(cst, rootCid)
		if err != nil {
			return err
		}

		lvm, err := vm.NewVM(rootCid, epoch, &vmRand{}, bs, mkFakedSigSyscalls(vm.Syscalls(ffiwrapper.ProofVerifier)))

		msg, err := types.DecodeMessage(tv.ApplyMessage)
		if err != nil {
			return err
		}

		ret, err := lvm.ApplyMessage(ctx, msg)
		if err != nil {
			return err
		}

		rval := ret.Return
		if rval == nil {
			rval = []byte{}
		}

		rootCid, err = lvm.Flush(ctx)
		if err != nil {
			return err
		}

		fmt.Println(rootCid)
		fmt.Println(tv.Post.StateTree.RootCID)
		fmt.Println(ret)

		return nil

	default:
		return fmt.Errorf("test vector class not supported")
	}
}

type vmRand struct {
}

func (*vmRand) GetRandomness(ctx context.Context, dst crypto.DomainSeparationTag, h abi.ChainEpoch, input []byte) ([]byte, error) {
	panic("implement me")
}

type fakedSigSyscalls struct {
	runtime.Syscalls
}

func (fss *fakedSigSyscalls) VerifySignature(_ crypto.Signature, _ address.Address, plaintext []byte) error {
	return nil
}

func mkFakedSigSyscalls(base vm.SyscallBuilder) vm.SyscallBuilder {
	return func(ctx context.Context, cstate *state.StateTree, cst cbor.IpldStore) runtime.Syscalls {
		return &fakedSigSyscalls{
			base(ctx, cstate, cst),
		}
	}
}
