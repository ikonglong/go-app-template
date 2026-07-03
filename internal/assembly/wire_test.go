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

// scaffold:begin
// TestContainerResolves_ImplChoice verifies the container is satisfiable with
// every persistence adapter the template ships. In a generated project both
// the test and the unused adapter are deleted, so this entire function is
// template scaffolding.
func TestContainerResolves_ImplChoice(t *testing.T) {
	tests := []struct {
		name     string
		repoImpl string
	}{
		{name: "jet", repoImpl: "jet"},
		{name: "sqlc", repoImpl: "sqlc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("REPO__IMPL", tt.repoImpl)

			c, err := BuildContainer(LoadConfig(""))
			if err != nil {
				t.Fatalf("BuildContainer: %v", err)
			}
			if err := c.Invoke(func(in runIn) {}); err != nil {
				t.Fatalf("Invoke: %v", err)
			}
		})
	}
}

// scaffold:end
