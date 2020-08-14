package builders

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"

	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-car"

	"github.com/filecoin-project/oni/tvx/lotus"
	"github.com/filecoin-project/oni/tvx/schema"
	ostate "github.com/filecoin-project/oni/tvx/state"
)

type Stage string

const (
	StagePreconditions = Stage("preconditions")
	StageApplies       = Stage("applies")
	StageChecks        = Stage("checks")
	StageFinished      = Stage("finished")
)

func init() {
	// disable logs, as we need a clean stdout output.
	log.SetOutput(ioutil.Discard)
	log.SetFlags(0)
}

// TODO use stage.Surgeon with non-proxying blockstore.
type Builder struct {
	Actors   *Actors
	Assert   *Asserter
	Messages *Messages

	vector schema.TestVector
	stage  Stage

	Wallet    *Wallet
	StateTree *state.StateTree

	stores *ostate.Stores
}

// MessageVector creates a builder for a message-class vector.
func MessageVector(metadata *schema.Metadata) *Builder {
	bs := blockstore.NewTemporary()
	cst := cbor.NewCborStore(bs)

	// Create a brand new state tree.
	st, err := state.NewStateTree(cst)
	if err != nil {
		panic(err)
	}

	b := &Builder{
		stage: StagePreconditions,
		stores: ostate.NewLocalStores(context.Background()),
	}

	b.Wallet = newWallet()
	b.StateTree = st
	b.Assert = newAsserter(b, StagePreconditions)
	b.Actors = &Actors{b: b}
	b.Messages = &Messages{b: b}

	b.vector.Class = schema.ClassMessage
	b.vector.Meta = metadata

	b.initializeZeroState()

	return b
}

func (b *Builder) CommitPreconditions() {
	if b.stage != StagePreconditions {
		panic("called CommitPreconditions at the wrong time")
	}

	// capture the preroot after applying all preconditions.
	preroot := b.FlushState()

	b.vector.Pre = &schema.Preconditions{
		Epoch:     0,
		StateTree: &schema.StateTree{RootCID: preroot},
	}

	b.stage = StageApplies
	b.Assert = newAsserter(b, StageApplies)
}

func (b *Builder) CommitApplies() {
	if b.stage != StageApplies {
		panic("called CommitApplies at the wrong time")
	}

	driver := lotus.NewDriver(context.Background())
	postroot := b.vector.Pre.StateTree.RootCID

	b.vector.Post = &schema.Postconditions{}
	for _, am := range b.Messages.All() {
		var err error
		am.Result, postroot, err = driver.ExecuteMessage(am.Message, postroot, b.stores.Blockstore, am.Epoch)
		b.Assert.NoError(err)

		// TODO do not replace the tree. Fix this.
		b.StateTree, err = state.LoadStateTree(b.stores.CBORStore, postroot)
		b.Assert.NoError(err)

		b.vector.ApplyMessages = append(b.vector.ApplyMessages, schema.Message{
			Bytes: MustSerialize(am.Message),
			Epoch: &am.Epoch,
		})
		b.vector.Post.Receipts = append(b.vector.Post.Receipts, &schema.Receipt{
			ExitCode:    am.Result.ExitCode,
			ReturnValue: am.Result.Return,
			GasUsed:     am.Result.GasUsed,
		})
	}

	b.vector.Post.StateTree = &schema.StateTree{RootCID: postroot}

	b.stage = StageChecks
	b.Assert = newAsserter(b, StageChecks)
}

func (b *Builder) Finish(w io.Writer) {
	if b.stage != StageChecks {
		panic("called Finish at the wrong time")
	}

	out := new(bytes.Buffer)
	gw := gzip.NewWriter(out)
	if err := b.WriteCAR(gw, b.vector.Pre.StateTree.RootCID, b.vector.Post.StateTree.RootCID); err != nil {
		panic(err)
	}
	if err := gw.Flush(); err != nil {
		panic(err)
	}
	if err := gw.Close(); err != nil {
		panic(err)
	}

	b.vector.CAR = out.Bytes()

	b.stage = StageFinished
	b.Assert = nil

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(b.vector); err != nil {
		panic(err)
	}
}

// WriteCAR recursively writes the tree referenced by the root as assert CAR into the
// supplied io.Writer.
//
// TODO use state.Surgeon instead. (This is assert copy of Surgeon#WriteCAR).
func (b *Builder) WriteCAR(w io.Writer, roots ...cid.Cid) error {
	carWalkFn := func(nd format.Node) (out []*format.Link, err error) {
		for _, link := range nd.Links() {
			if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
				continue
			}
			out = append(out, link)
		}
		return out, nil
	}

	return car.WriteCarWithWalker(context.Background(), b.stores.DAGService, roots, w, carWalkFn)
}

func (b *Builder) FlushState() cid.Cid {
	preroot, err := b.StateTree.Flush(context.Background())
	if err != nil {
		panic(err)
	}
	return preroot
}
