package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lb "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	cbor "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"
)

func cmdGenerate(c *cli.Context) error {
	repoDir := c.String("repodir")
	if repoDir == "" {
		return errors.New("repodir is blank")
	}

	outputFile := c.String("output")
	if outputFile == "" {
		return errors.New("output is blank")
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

	bs := lb.NewBlockstore(ds)
	//bs := blockstore.NewBlockstore(ds)

	cst := store.NewChainStore(bs, mds, mkFakedSigSyscalls(vm.Syscalls(ffiwrapper.ProofVerifier)))

	err = cst.Load()
	if err != nil {
		return err
	}

	fi, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer func() {
		err := fi.Close()
		if err != nil {
			fmt.Printf("error closing output file: %+v", err)
		}
	}()

	fi2, err := os.Create("statetree.car")
	if err != nil {
		return err
	}
	defer func() {
		err := fi2.Close()
		if err != nil {
			fmt.Printf("error closing output file: %+v", err)
		}
	}()

	ctx := context.Background()

	//cur := cst.GetHeaviestTipSet()
	//if cur == nil {
	//return errors.New("heaviest tipset is nil")
	//}

	//err = cst.SetHead(cur)
	//if err != nil {
	//return err
	//}

	tsk := types.EmptyTSK
	ts, err := cst.GetTipSetFromKey(tsk)
	if err != nil {
		return err
	}

	h := abi.ChainEpoch(5)
	cur, err := cst.GetTipsetByHeight(ctx, h, ts, true)
	if err != nil {
		return err
	}

	log.Infof("Tipset key: %s", cur.Key())
	log.Infof("Tipset height: %d", cur.Height())

	stm := stmgr.NewStateManager(cst)

	stateroot, _, err := stm.TipSetState(ctx, cur)
	if err != nil {
		return err
	}

	store := adt.WrapStore(ctx, cbor.NewCborStore(bs))
	emptyroot, err := adt.MakeEmptyArray(store).Root()
	if err != nil {
		return fmt.Errorf("amt build failed: %w", err)
	}

	mm := &types.MsgMeta{
		BlsMessages:   emptyroot,
		SecpkMessages: emptyroot,
	}
	mmb, err := mm.ToStorageBlock()
	if err != nil {
		return fmt.Errorf("serializing msgmeta failed: %w", err)
	}
	if err := bs.Put(mmb); err != nil {
		return fmt.Errorf("putting msgmeta block to blockstore: %w", err)
	}

	log.Infof("Empty Genesis root: %s", emptyroot)
	log.Infof("State root: %s", stateroot)
	log.Infof("Messages: %s", mmb.Cid())

	genesisticket := &types.Ticket{
		VRFProof: []byte("vrf proof0000000vrf proof0000000"),
	}

	b := &types.BlockHeader{
		Miner:                 builtin.SystemActorAddr,
		Ticket:                genesisticket,
		Parents:               []cid.Cid{},
		Height:                0,
		ParentWeight:          types.NewInt(0),
		ParentStateRoot:       stateroot,
		Messages:              mmb.Cid(),
		ParentMessageReceipts: emptyroot,
		BLSAggregate:          nil,
		BlockSig:              nil,
		Timestamp:             uint64(time.Now().UnixNano()),
		ElectionProof:         new(types.ElectionProof),
		BeaconEntries: []types.BeaconEntry{
			{
				Round: 0,
				Data:  make([]byte, 32),
			},
		},
	}

	sb, err := b.ToStorageBlock()
	if err != nil {
		return fmt.Errorf("serializing block header failed: %w", err)
	}

	log.Infof("Block header: %s\n", sb.Cid())

	if err := bs.Put(sb); err != nil {
		return fmt.Errorf("putting header to blockstore: %w", err)
	}

	// serialise block
	//var bb2 bytes.Buffer
	//writer2 := bufio.NewWriter(&bb2)

	offl := offline.Exchange(bs)
	blkserv := blockservice.New(bs, offl)
	dserv := merkledag.NewDAGService(blkserv)

	if err := car.WriteCarWithWalker(ctx, dserv, []cid.Cid{b.Cid()}, fi, walker); err != nil {
		//if err := car.WriteCarWithWalker(ctx, dserv, []cid.Cid{b.Cid()}, writer2, walker); err != nil {
		return fmt.Errorf("failed to write car file: %w", err)
	}

	// serialise statetree
	//var bb bytes.Buffer
	//writer := bufio.NewWriter(&bb)

	offl = offline.Exchange(bs)
	blkserv = blockservice.New(bs, offl)
	dserv = merkledag.NewDAGService(blkserv)

	if err := car.WriteCarWithWalker(ctx, dserv, []cid.Cid{stateroot}, fi2, walker); err != nil {
		return fmt.Errorf("failed to write car file: %w", err)
	}

	//str := hex.EncodeToString(bb.Bytes())

	//fmt.Println("stateroot:", stateroot)
	//fmt.Println("statetree:", str)

	// sanity check
	//bss := blockstore.NewBlockstore(ds)
	//_, err = car.LoadCar(bss, bytes.NewReader(bb2.Bytes()))
	//fi.Seek(0, 0)
	//_, err = car.LoadCar(bss, fi)
	//if err != nil {
	//return fmt.Errorf("error loading car file failed: %w", err)
	//}

	return nil
}

//var walker = car.DefaultWalkFunc

func walker(nd format.Node) (out []*format.Link, err error) {
	for _, link := range nd.Links() {
		//spew.Dump(link)
		if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
			continue
		}
		out = append(out, link)
	}

	return out, nil
}
