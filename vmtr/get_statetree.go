package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ipld/go-car"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lb "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	"github.com/ipfs/go-merkledag"
	"github.com/urfave/cli/v2"
)

func cmdGetStatetree(c *cli.Context) error {
	repoDir := c.String("repodir")
	if repoDir == "" {
		return errors.New("repodir is blank")
	}

	output := c.String("outputfile")
	if output == "" {
		return errors.New("outputfile is blank")
	}

	height := c.Int("height") // 0 means heaviest

	r, err := repo.NewFS(repoDir)
	if err != nil {
		return err
	}

	err = r.Init(repo.StorageMiner)
	if err != nil {
		return err
	}

	lr, err := r.Lock(repo.FullNode)
	if err != nil {
		return err
	}
	defer lr.Close() //nolint:errcheck

	ds, err := lr.Datastore("/chain")
	if err != nil {
		return err
	}

	mds, err := lr.Datastore("/metadata")
	if err != nil {
		return err
	}

	bs := lb.NewBlockstore(ds)

	cst := store.NewChainStore(bs, mds, mkFakedSigSyscalls(vm.Syscalls(ffiwrapper.ProofVerifier)))

	err = cst.Load()
	if err != nil {
		return err
	}

	var cur *types.TipSet
	ctx := context.Background()

	if height == 0 {
		cur = cst.GetHeaviestTipSet()
		if cur == nil {
			return errors.New("heaviest tipset is nil")
		}
	} else {
		tsk := types.EmptyTSK
		ts, err := cst.GetTipSetFromKey(tsk)
		if err != nil {
			return err
		}

		cur, err = cst.GetTipsetByHeight(ctx, abi.ChainEpoch(height), ts, true)
		if err != nil {
			return err
		}
	}

	log.Infof("Tipset key: %s", cur.Key())
	log.Infof("Tipset height: %d", cur.Height())

	// get stateroot
	stm := stmgr.NewStateManager(cst)

	stateroot, _, err := stm.TipSetState(ctx, cur)
	if err != nil {
		return err
	}

	log.Infof("Stateroot: %s", stateroot)

	var b bytes.Buffer

	err = serialise(bs, stateroot, &b)
	if err != nil {
		return err
	}

	o, err := os.Create(output)
	if err != nil {
		return err
	}
	defer func() {
		err := o.Close()
		if err != nil {
			fmt.Printf("error closing output file: %+v", err)
		}
	}()

	_, err = o.Write([]byte(hex.EncodeToString(b.Bytes())))
	if err != nil {
		return err
	}

	return nil
}

func serialise(bs blockstore.Blockstore, c cid.Cid, w io.Writer) error {
	ctx := context.Background()

	offl := offline.Exchange(bs)
	blkserv := blockservice.New(bs, offl)
	dserv := merkledag.NewDAGService(blkserv)

	if err := car.WriteCarWithWalker(ctx, dserv, []cid.Cid{c}, w, walker); err != nil {
		return fmt.Errorf("failed to write car file: %w", err)
	}

	return nil
}
