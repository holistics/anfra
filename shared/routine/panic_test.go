package routine

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// A panic in a routine without Recover reaches crashOnPanic, which would crash
// the process; here, repanic hands it to the test instead.
func TestPanicCrashes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		register func(g *Group)
	}{
		{"every", func(g *Group) {
			g.Every("every.panics", time.Hour, func(context.Context) error { panic("kaboom") }, Immediately())
		}},
		{"loop", func(g *Group) {
			g.Loop("loop.panics", func(context.Context) error { panic("kaboom") })
		}},
		{"worker", func(g *Group) {
			g.Worker("worker.panics", func(context.Context) { panic("kaboom") })
		}},
		{"task", func(g *Group) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			crashed := make(chan any, 1)
			repanic = func(v any) { crashed <- v }
			t.Cleanup(func() { repanic = func(v any) { panic(v) } })

			g := NewGroup("test", slog.New(slog.DiscardHandler))
			tc.register(g)
			g.Start(context.Background())
			if tc.name == "task" {
				g.Task("task.panics", func(context.Context) error { panic("kaboom") })
			}
			select {
			case v := <-crashed:
				if v != "kaboom" {
					t.Errorf("crashed with %v, want the routine's panic", v)
				}
			case <-time.After(time.Second):
				t.Fatal("the panic did not crash")
			}
			if err := g.Stop(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
}
