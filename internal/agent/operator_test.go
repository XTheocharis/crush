package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskSignatureDeterminism(t *testing.T) {
	t.Parallel()

	context := map[string]string{
		"z_key": "z_value",
		"a_key": "a_value",
		"m_key": "m_value",
		"b_key": "b_value",
	}

	var first string
	for i := range 100 {
		sig := taskSignature("test-task", context)
		if i == 0 {
			first = sig
		}
		require.Equal(t, first, sig, "taskSignature produced different output on iteration %d", i)
	}
}
