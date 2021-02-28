package topology

import "testing"

func TestDeviceFunctionRoundtrips(t *testing.T) {
	for i := DeviceFunction(1); i < NoFunction; i++ {
		if j := deviceFunctionFromString(i.String()); j != i {
			t.Errorf("function %s: got %d, want %d", i.String(), j, i)
		}
	}
}
