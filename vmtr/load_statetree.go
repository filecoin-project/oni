package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/davecgh/go-spew/spew"
	"github.com/ipld/go-car"

	"github.com/filecoin-project/lotus/chain/state"
	lb "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
)

func cmdLoadStatetree(c *cli.Context) error {
	input := c.String("inputfile")
	if input == "" {
		return errors.New("inputfile is blank")
	}

	stateroot := c.String("stateroot")
	if stateroot == "" {
		return errors.New("stateroot is blank")
	}

	hexbytes, err := ioutil.ReadFile(input)
	if err != nil {
		return err
	}

	b, err := hex.DecodeString(string(hexbytes))
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

	_, err = car.LoadCar(bs, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("error loading car: %w", err)
	}

	sr, err := cid.Decode(stateroot)
	if err != nil {
		return fmt.Errorf("error decoding stateroot: %w", err)
	}

	cst := cbor.NewCborStore(bs)
	statetree, err := state.LoadStateTree(cst, sr)
	if err != nil {
		return fmt.Errorf("error loading state tree: %w", err)
	}

	spew.Dump(statetree)

	return nil
}
