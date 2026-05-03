package notifier

import (
	"context"
	"io"
	"os"

	"github.com/rifat977/taplytix/internal/alert"
)

// Bell writes the ASCII BEL character to a sink (os.Stderr by default) when
// an alert fires. Tests may inject a custom writer.
type Bell struct {
	W io.Writer
}

func NewBell() *Bell { return &Bell{W: os.Stderr} }

func (b *Bell) Name() string { return "bell" }

func (b *Bell) Notify(_ context.Context, _ alert.Alert) error {
	w := b.W
	if w == nil {
		w = os.Stderr
	}
	_, err := w.Write([]byte{0x07}) // BEL
	return err
}
