package main

import (
	"github.com/urfave/cli/v2"
)

var execLotusCmd = &cli.Command{
	Name:        "exec-lotus",
	Description: "execute a test vector against Lotus",
	Flags:       []cli.Flag{&fileFlag},
	Action:      runExecLotus,
}

func runExecLotus(c *cli.Context) error {
	// f := c.String("file")
	// if f == "" {
	// 	return fmt.Errorf("test vector file cannot be empty")
	// }
	//
	// file, err := os.Open(f)
	// if err != nil {
	// 	return fmt.Errorf("failed to open test vector: %w", err)
	// }
	//
	// var (
	// 	dec = json.NewDecoder(file)
	// 	tv  TestVector
	// )
	//
	// if err = dec.Decode(&tv); err != nil {
	// 	return fmt.Errorf("failed to decode test vector: %w", err)
	// }
	//
	// switch tv.Class {
	// case "message":
	// 	var (
	// 		ctx   = context.TODO()
	// 		epoch = tv.Pre.Epoch
	// 		root  = tv.Pre.StateTree.RootCID
	// 	)
	//
	// 	bs := blockstore.NewTemporary()
	//
	// 	header, err := car.LoadCar(bs, bytes.NewReader(tv.Pre.StateTree.CAR))
	// 	if err != nil {
	// 		return fmt.Errorf("failed to load state tree car from test vector: %w", err)
	// 	}
	//
	// 	fmt.Println(header.Roots)
	//
	// 	cst := cbor.NewCborStore(bs)
	// 	rootCid, err := cid.Decode(root)
	// 	if err != nil {
	// 		return err
	// 	}
	//
	// 	// Load the state tree.
	// 	st, err := state.LoadStateTree(cst, rootCid)
	// 	if err != nil {
	// 		return err
	// 	}
	//
	// 	_ = fmt.Sprint(st)
	//
	// 	initActor, err := st.GetActor(builtin.InitActorAddr)
	// 	if err != nil {
	// 		return fmt.Errorf("failed to resolve init actor: %w", err)
	// 	}
	//
	// 	spew.Dump(initActor)
	//
	// 	addr, err := address.NewFromString("t3vxjqfuuxayx26cetvhdmsjwl2mss6uwiiv6t4ssmdcloucy6b4t5jfy5j56sdnj2bw7ypkm7p6f4ud2fleoq")
	// 	if err != nil {
	// 		return err
	// 	}
	//
	// 	actor, err := st.GetActor(addr)
	// 	if err != nil {
	// 		return fmt.Errorf("failed to find actor: %w", err)
	// 	}
	//
	// 	spew.Dump(actor)
	//
	// 	fmt.Println("decoding message")
	// 	msg, err := types.DecodeMessage(tv.ApplyMessage)
	// 	if err != nil {
	// 		return err
	// 	}
	//
	// 	fmt.Println(rootCid)
	// 	fmt.Println(tv.Post.StateTree.RootCID)
	// 	fmt.Println(ret)
	//
	// 	return nil
	//
	// default:
	// 	return fmt.Errorf("test vector class not supported")
	// }
	return nil
}
