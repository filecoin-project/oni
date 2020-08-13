package builders

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"

	"github.com/filecoin-project/lotus/chain/vm"

	"github.com/filecoin-project/oni/tvx/lotus"
	"github.com/filecoin-project/oni/tvx/schema"
)

// PrepareFn safe to panic.
type PrepareFn func(pb *Preconditions)

// MessagesFn safe to panic.
type MessagesFn func(m *Messages)

// CheckFunc safe to panic.
type CheckFunc func(ma *StateChecker, rets []*vm.ApplyRet)

// GenerateMessageVector produces assert message vector on stdout.
func GenerateMessageVector(metadata *schema.Metadata, precondictionFn PrepareFn, messagesFn MessagesFn, checkFn CheckFunc) {
	tv := &schema.TestVector{
		Class: schema.ClassMessage,
		Meta:  metadata,
		Pre: &schema.Preconditions{
			StateTree: &schema.StateTree{},
		},
		Post: &schema.Postconditions{
			StateTree: &schema.StateTree{},
		},
	}

	b := NewBuilder()

	precondictionFn(&Preconditions{b})

	// capture the preroot after applying all preconditions.
	preroot := b.FlushState()

	msgs := NewMessages()
	messagesFn(msgs)

	driver := lotus.NewDriver(context.Background())
	postroot := preroot

	var tvmsgs []schema.Message
	var rets []*vm.ApplyRet
	for _, am := range msgs.All() {
		ret, root, err := driver.ExecuteMessage(am.Message, postroot, b.bs, am.Epoch)
		if err != nil {
			panic(err)
		}

		rets = append(rets, ret)
		tvmsgs = append(tvmsgs, schema.Message{
			Bytes: MustSerialize(am.Message),
			Epoch: &am.Epoch,
		})

		postroot = root
	}

	sc := &StateChecker{st: b.StateTree}
	checkFn(sc, rets)

	out := new(bytes.Buffer)
	gw := gzip.NewWriter(out)
	if err := b.WriteCAR(gw, preroot, postroot); err != nil {
		panic(err)
	}
	if err := gw.Flush(); err != nil {
		panic(err)
	}
	if err := gw.Close(); err != nil {
		panic(err)
	}

	tv.Pre.StateTree.RootCID = preroot
	tv.Post.StateTree.RootCID = postroot
	tv.CAR = out.Bytes()
	tv.ApplyMessages = tvmsgs

	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(tv); err != nil {
		panic(err)
	}
}
