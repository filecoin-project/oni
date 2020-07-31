package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/urfave/cli/v2"
)

func cmdExport(c *cli.Context) error {
	repoDir := c.String("repodir")
	if repoDir == "" {
		return errors.New("repodir is blank")
	}

	output := c.String("outputfile")
	if output == "" {
		return errors.New("outputfile is blank")
	}

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

	bs := blockstore.NewBlockstore(ds)

	cst := store.NewChainStore(bs, mds, mkFakedSigSyscalls(vm.Syscalls(ffiwrapper.ProofVerifier)))

	err = cst.Load()
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

	ts := cst.GetHeaviestTipSet()
	if ts == nil {
		return errors.New("heaviest tipset is nil")
	}

	ctx := context.Background()
	stream, err := chainExport(ctx, cst, ts)
	if err != nil {
		return err
	}

	for b := range stream {
		_, err := o.Write(b)
		if err != nil {
			return err
		}
	}

	return nil
}

func chainExport(ctx context.Context, cst *store.ChainStore, ts *types.TipSet) (<-chan []byte, error) {
	r, w := io.Pipe()
	out := make(chan []byte)
	go func() {
		defer w.Close() //nolint:errcheck // it is a pipe
		if err := cst.Export(ctx, ts, w); err != nil {
			log.Errorf("chain export call failed: %s", err)
			return
		}
	}()

	go func() {
		defer close(out)
		for {
			buf := make([]byte, 4096)
			n, err := r.Read(buf)
			if err != nil && err != io.EOF {
				log.Errorf("chain export pipe read failed: %s", err)
				return
			}
			select {
			case out <- buf[:n]:
			case <-ctx.Done():
				log.Warnf("export writer failed: %s", ctx.Err())
			}
			if err == io.EOF {
				return
			}
		}
	}()

	return out, nil
}
