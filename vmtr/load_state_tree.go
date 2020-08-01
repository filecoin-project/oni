package main

import (
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/ipld/go-car"

	"github.com/filecoin-project/lotus/chain/state"
	lb "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
)

func cmdLoadStateTree(c *cli.Context) error {
	fi, err := os.Open("statetree.car")
	if err != nil {
		return err
	}

	r := repo.NewMemory(nil)

	lr, err := r.Lock(repo.FullNode)
	if err != nil {
		return err
	}
	defer lr.Close() //nolint:errcheck

	ds, err := lr.Datastore("/chain")
	if err != nil {
		return err
	}

	bs := lb.NewBlockstore(ds)

	//fi, err := hex.DecodeString(statetreefull)
	//if err != nil {
	//return err
	//}
	//reader := bytes.NewReader(fi)

	_, err = car.LoadCar(bs, fi)
	if err != nil {
		return fmt.Errorf("error loading car: %w", err)
	}

	stateroot, _ := cid.Decode("bafy2bzacecwokparndcnk7gwyebub6tfdpz3nobvk3kapzqyxywgef5lywlby")

	cst := cbor.NewCborStore(bs)
	statetree, err := state.LoadStateTree(cst, stateroot)
	if err != nil {
		return fmt.Errorf("error loading state tree: %w", err)
	}

	spew.Dump(statetree)

	return nil
}
