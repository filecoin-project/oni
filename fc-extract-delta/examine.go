package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/fc-extract-delta/lib"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func examine(c *cli.Context) error {
	input, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	rawInput, err := hex.DecodeString(trimQuotes(strings.TrimSpace(string(input))))
	if err != nil {
		return err
	}

	tree, err := lib.RecoverStateTree(context.TODO(), rawInput)
	if err != nil {
		return err
	}

	initActor, err := tree.GetActor(builtin.InitActorAddr)
	if err != nil {
		return err
	}

	var ias init_.State
	if err := tree.Store.Get(context.TODO(), initActor.Head, &ias); err != nil {
		return err
	}
	adtStore := adt.WrapStore(context.TODO(), tree.Store)
	m, err := adt.AsMap(adtStore, ias.AddressMap)
	if err != nil {
		return err
	}
	actors, err := m.CollectKeys()
	for _, actor := range actors {
		fmt.Printf("%s\n", actor)
	}

	return nil
}
