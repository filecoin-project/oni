package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/oni/tvx/schema"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"

	"github.com/ipfs/go-cid"
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
		tv  schema.TestVector
	)

	if err = dec.Decode(&tv); err != nil {
		return fmt.Errorf("failed to decode test vector: %w", err)
	}

	switch tv.Class {
	case "message":
		var (
			ctx   = context.TODO()
			epoch = tv.Pre.Epoch
			root  = tv.Pre.StateTree.RootCID
		)

		bs := blockstore.NewTemporary()

		header, err := car.LoadCar(bs, bytes.NewReader(tv.Pre.StateTree.CAR))
		if err != nil {
			return fmt.Errorf("failed to load state tree car from test vector: %w", err)
		}

		fmt.Println(header.Roots)

		cst := cbor.NewCborStore(bs)
		rootCid, err := cid.Decode(root)
		if err != nil {
			return err
		}

		// Load the state tree.
		st, err := state.LoadStateTree(cst, rootCid)
		if err != nil {
			return err
		}

		_ = fmt.Sprint(st)

		addr, err := address.NewFromString("t3vxjqfuuxayx26cetvhdmsjwl2mss6uwiiv6t4ssmdcloucy6b4t5jfy5j56sdnj2bw7ypkm7p6f4ud2fleoq")
		if err != nil {
			return err
		}

		actor, err := st.GetActor(addr)
		if err != nil {
			return fmt.Errorf("failed to find actor: %w", err)
		}

		spew.Dump(actor)

		fmt.Println("creating vm")
		lvm, err := vm.NewVM(header.Roots[0], epoch, &vmRand{}, bs, mkFakedSigSyscalls(vm.Syscalls(ffiwrapper.ProofVerifier)), nil)

		fmt.Println("decoding message")
		msg, err := types.DecodeMessage(tv.ApplyMessage)
		if err != nil {
			return err
		}

		fmt.Println("applying message")

		ret, err := lvm.ApplyMessage(ctx, msg)
		if err != nil {
			return err
		}

		fmt.Printf("applied message: %+v\n", ret)

		rval := ret.Return
		if rval == nil {
			rval = []byte{}
		}

		fmt.Println("flushing")
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
