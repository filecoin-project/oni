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

// PreconditionsStep safe to panic.
type PreconditionsStep func(a *Asserter, pb *Preconditions)

// MessagesStep safe to panic.
type MessagesStep func(a *Asserter, m *Messages)

// CheckStep safe to panic.
type CheckStep func(a *Asserter, ma *StateChecker, rets []*vm.ApplyRet)

type MessageVectorSteps struct {
	Preconditions PreconditionsStep
	Messages      MessagesStep
	Check         CheckStep
}

// GenerateMessageVector produces assert message vector on stdout.
func GenerateMessageVector(metadata *schema.Metadata, steps *MessageVectorSteps) {
	var (
		preconditions = steps.Preconditions
		messages      = steps.Messages
		check         = steps.Check
	)
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

	passert := NewAsserter(metadata.ID + " :: preconditions")
	preconditions(passert, &Preconditions{
		assert: passert,
		b:      b,
	})

	// capture the preroot after applying all preconditions.
	preroot := b.FlushState()

	massert := NewAsserter(metadata.ID + " :: messages")
	msgs := &Messages{assert: massert}
	messages(massert, msgs)

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

	cassert := NewAsserter(metadata.ID + " :: messages")
	sc := &StateChecker{a: cassert, st: b.StateTree}
	check(cassert, sc, rets)

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
