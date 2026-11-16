package interact

import (
	"context"
	"testing"
)

type yes struct{ asked string }

func (y *yes) Confirm(_ context.Context, m string) (bool, error) { y.asked = m; return true, nil }

type rec struct{ msgs []string }

func (r *rec) Report(_ context.Context, _, _ float64, m string) { r.msgs = append(r.msgs, m) }

func TestContextCarriesImplementations(t *testing.T) {
	ctx := context.Background()
	if _, ok := ConfirmerFrom(ctx); ok {
		t.Fatal("empty context has a confirmer")
	}
	Report(ctx, 1, 2, "no-op without a Progress")

	y, r := &yes{}, &rec{}
	ctx = WithProgress(WithConfirmer(ctx, y), r)
	c, ok := ConfirmerFrom(ctx)
	if !ok {
		t.Fatal("confirmer lost")
	}
	if got, err := c.Confirm(ctx, "stop?"); !got || err != nil || y.asked != "stop?" {
		t.Fatalf("confirm %v %v %q", got, err, y.asked)
	}
	Report(ctx, 1, 2, "half")
	if len(r.msgs) != 1 || r.msgs[0] != "half" {
		t.Fatalf("progress %v", r.msgs)
	}
}
