package main

import (
	"fmt"
	"os"
)

var t dummyTestingT

type dummyTestingT struct{}

func (d dummyTestingT) FailNow() {
	panic("fail now")
}

func (d dummyTestingT) Errorf(format string, args ...interface{}) {
	fmt.Printf(format, args)
	os.Exit(1)
}
