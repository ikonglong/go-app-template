package assembly

import "testing"

// TestContainerResolves proves the dig graph is satisfiable end-to-end.
// dig.Provide only registers — missing types surface at Invoke time, so
// without this test a misnamed dependency would only fail at first boot.
// A no-op Invoke that depends on the same runIn set Run uses exercises
// every path Run does without touching the network.
func TestContainerResolves(t *testing.T) {
	c, err := BuildContainer(LoadConfig(""))
	if err != nil {
		t.Fatalf("BuildContainer: %v", err)
	}
	if err := c.Invoke(func(in runIn) {}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}
