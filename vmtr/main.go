package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
)

var chainfile = flag.String("chainfile", "chain.car", "location of chain file")

var log = logging.Logger("main")

func init() {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	_ = logging.SetLogLevel("*", "INFO")

	build.InsecurePoStValidation = true
	build.DisableBuiltinAssets = true

	power.ConsensusMinerMinPower = big.NewInt(2048)
	miner.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)

	// MessageConfidence is the amount of tipsets we wait after a message is
	// mined, e.g. payment channel creation, to be considered committed.
	//build.MessageConfidence = 1

	// The period over which all a miner's active sectors will be challenged.
	miner.WPoStProvingPeriod = abi.ChainEpoch(240) // instead of 24 hours

	// The duration of a deadline's challenge window, the period before a deadline when the challenge is available.
	miner.WPoStChallengeWindow = abi.ChainEpoch(5) // instead of 30 minutes (still 48 per day)

	// Number of epochs between publishing the precommit and when the challenge for interactive PoRep is drawn
	// used to ensure it is not predictable by miner.
	miner.PreCommitChallengeDelay = abi.ChainEpoch(10)
}

func main() {
	flag.Parse()

	err := func() error {
		repoDir, err := ioutil.TempDir("", "repo-dir")
		if err != nil {
			return err
		}

		r, err := repo.NewFS(repoDir)
		if err != nil {
			return err
		}

		err = r.Init(repo.StorageMiner)
		if err != nil {
			return err
		}

		if err := importChain(r, *chainfile); err != nil {
			return err
		}

		return nil
	}()
	if err != nil {
		log.Fatal(err.Error())
	}
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
