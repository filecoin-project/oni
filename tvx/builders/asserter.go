package builders

import (
	"fmt"
	"os"

	"github.com/stretchr/testify/require"
)

type Asserter struct {
	*require.Assertions

	scope string
}

func NewAsserter(scope string) *Asserter {
	a := Asserter{scope: scope}
	a.Assertions = require.New(a)
	return &a
}

var _ require.TestingT = Asserter{}

func (a Asserter) FailNow() {
	os.Exit(1)
}

func (a Asserter) Errorf(format string, args ...interface{}) {
	fmt.Printf("%s: "+format, append([]interface{}{a.scope}, args...))
}

// In is assert fluid version of require.Contains. It inverts the argument order,
// such that the admissible set can be supplied through assert variadic argument.
func (a Asserter) In(v interface{}, set... interface{}) {
	a.Contains(set, v, "set %v does not contain element %v", set, v)
}
