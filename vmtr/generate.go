package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
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

	bs := blockstore.NewBlockstore(ds)

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

	ctx := context.Background()

	cur := cst.GetHeaviestTipSet()
	if cur == nil {
		return errors.New("heaviest tipset is nil")
	}

	//tsk := types.EmptyTSK
	//ts, err := cst.GetTipSetFromKey(tsk)
	//if err != nil {
	//return err
	//}

	//h := abi.ChainEpoch(5)
	//cur, err := cst.GetTipsetByHeight(ctx, h, ts, true)
	//if err != nil {
	//return err
	//}

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

	// BEGIN MISSING KEYS/VALUES

	//root@tg-lotus-soup-b455c159e1af-miners-0:/# lll chain read-obj bafkqaetgnfwc6mjpon2g64tbm5sw22lomvza
	//66696c2f312f73746f726167656d696e6572

	magicBlocks := []struct {
		Cid  string
		Data string
	}{
		{
			"bafkqadlgnfwc6mjpmfrwg33vnz2a",
			"66696c2f312f6163636f756e74",
		},
		{
			"bafkqaetgnfwc6mjpon2g64tbm5sw22lomvza",
			"66696c2f312f73746f726167656d696e6572",
		},
		{
			"bafkqaftgnfwc6mjpozsxe2lgnfswi4tfm5uxg5dspe",
			"66696c2f312f76657269666965647265676973747279",
		},
		{
			"bafkqaetgnfwc6mjpon2g64tbm5sxa33xmvza",
			"66696c2f312f73746f72616765706f776572",
		},
		{
			"bafkqactgnfwc6mjpmnzg63q",
			"66696c2f312f63726f6e",
		},
		{
			"bafkqaddgnfwc6mjpon4xg5dfnu",
			"66696c2f312f73797374656d",
		},
		{
			"bafkqae3gnfwc6mjpon2g64tbm5sw2ylsnnsxi",
			"66696c2f312f73746f726167656d61726b6574",
		},
		{
			"bafkqactgnfwc6mjpnfxgs5a",
			"66696c2f312f696e6974",
		},
		{
			"bafkqaddgnfwc6mjpojsxoylsmq",
			"66696c2f312f726577617264",
		},
	}

	for _, v := range magicBlocks {
		magicdata, err := hex.DecodeString(v.Data)
		if err != nil {
			panic(err)
		}
		fmt.Printf("% x", magicdata)

		magiccid, err := cid.Decode(v.Cid)
		if err != nil {
			return err
		}
		magicblock, err := blocks.NewBlockWithCid(magicdata, magiccid)
		if err != nil {
			return err
		}

		if err := bs.Put(magicblock); err != nil {
			return fmt.Errorf("putting magicblock to blockstore: %w", err)
		}

		log.Infof("Magic block: %s\n", magicblock.Cid())
	}

	// END MISSING KEYS/VALUES

	log.Infof("Block header: %s\n", sb.Cid())

	if err := bs.Put(sb); err != nil {
		return fmt.Errorf("putting header to blockstore: %w", err)
	}

	offl := offline.Exchange(bs)
	blkserv := blockservice.New(bs, offl)
	dserv := merkledag.NewDAGService(blkserv)

	if err := car.WriteCarWithWalker(ctx, dserv, []cid.Cid{b.Cid()}, fi, walker); err != nil {
		return fmt.Errorf("failed to write car file: %w", err)
	}

	return nil
}

func walker(nd format.Node) (out []*format.Link, err error) {
	for _, link := range nd.Links() {
		spew.Dump(link)
		if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
			continue
		}
		out = append(out, link)
	}

	return out, nil
}
