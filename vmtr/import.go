package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
)

func cmdImport(c *cli.Context) error {
	var err error
	repoDir := c.String("repodir")
	if repoDir == "" {
		repoDir, err = ioutil.TempDir("", "repo-dir")
		if err != nil {
			return err
		}
	}

	r, err := repo.NewFS(repoDir)
	if err != nil {
		return err
	}

	err = r.Init(repo.StorageMiner)
	if err != nil {
		return err
	}

	if err := importChain(r, c.String("chainfile")); err != nil {
		return err
	}

	log.Infof("repo dir: %s", repoDir)

	return nil
}

func importChain(r repo.Repo, fname string) error {
	fi, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer fi.Close() //nolint:errcheck

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

	log.Info("importing chain from file...")
	ts, err := cst.Import(fi)
	if err != nil {
		return fmt.Errorf("importing chain failed: %w", err)
	}

	stm := stmgr.NewStateManager(cst)

	log.Infof("validating imported chain...")
	if err := stm.ValidateChain(context.TODO(), ts); err != nil {
		return fmt.Errorf("chain validation failed: %w", err)
	}

	log.Info("accepting %s as new head", ts.Cids())
	if err := cst.SetHead(ts); err != nil {
		return err
	}

	return nil
}

type fakedSigSyscalls struct {
	runtime.Syscalls
}

func (fss *fakedSigSyscalls) VerifySignature(signature crypto.Signature, signer address.Address, plaintext []byte) error {
	return nil
}

func mkFakedSigSyscalls(base vm.SyscallBuilder) vm.SyscallBuilder {
	return func(ctx context.Context, cstate *state.StateTree, cst cbor.IpldStore) runtime.Syscalls {
		return &fakedSigSyscalls{
			base(ctx, cstate, cst),
		}
	}
}
